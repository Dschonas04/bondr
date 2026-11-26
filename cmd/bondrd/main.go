// bondrd ist der Kern von Bondr: Lastverteiler über alle Internetzugänge, lokaler Proxy
// und Downloader. Die Apps für Windows, macOS und Linux starten ihn und steuern ihn über eine
// lokale Schnittstelle.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Dschonas04/bondr/internal/api"
	"github.com/Dschonas04/bondr/internal/einstellungen"
	"github.com/Dschonas04/bondr/internal/laden"
	"github.com/Dschonas04/bondr/internal/proxy"
	"github.com/Dschonas04/bondr/internal/system"
	"github.com/Dschonas04/bondr/internal/umleitung"
	"github.com/Dschonas04/bondr/internal/verbrauch"
	"github.com/Dschonas04/bondr/internal/verteiler"
)

// Version wird beim Bau gesetzt.
var Version = "dev"

func main() {
	apiPort := flag.Int("api", 47801, "Port der Steuerung")
	proxyPort := flag.Int("proxy", 1080, "Port des Proxys (SOCKS5 und HTTP)")
	eltern := flag.Int("eltern", 0, "Prozessnummer der App; endet sie, endet auch bondrd")
	version := flag.Bool("version", false, "Version ausgeben")
	umleiten := flag.Bool("umleitung", false, "Verbindungen der eingestellten Programme abfangen (Windows, Adminrechte)")
	einrichten := flag.Bool("umleitung-einrichten", false, "Aufgabe für die Umleitung anlegen und starten (Adminrechte)")
	mitTunnel := flag.Bool("tunnel", false, "Verkehr aus dem VPN-Tunnel der App verteilen (Android)")
	flag.Parse()
	if *version {
		fmt.Println(Version)
		return
	}

	ordner := einstellungen.Ordner()
	os.MkdirAll(ordner, 0o700)
	logName := "bondrd.log"
	if *umleiten || *einrichten {
		logName = "umleitung.log"
	}
	if f, err := os.OpenFile(filepath.Join(ordner, logName), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
		log.SetOutput(f)
	}
	if *einrichten {
		if err := umleitung.Einrichten(); err != nil {
			log.Printf("Einrichten: %v", err)
			os.Exit(1)
		}
		return
	}
	if *umleiten {
		if err := umleitung.Laufen(fmt.Sprintf("127.0.0.1:%d", *proxyPort), fmt.Sprintf("127.0.0.1:%d", *apiPort)); err != nil {
			log.Printf("Umleitung: %v", err)
			os.Exit(1)
		}
		return
	}

	apiL, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *apiPort))
	if err != nil {
		log.Printf("Steuerung belegt, läuft schon: %v", err)
		return
	}
	proxyAdresse := fmt.Sprintf("127.0.0.1:%d", *proxyPort)
	proxyL, err := net.Listen("tcp", proxyAdresse)
	if err != nil {
		log.Printf("Proxy-Port %s belegt: %v", proxyAdresse, err)
	}

	e := einstellungen.Laden()
	v := verteiler.Neu(e)
	ctx, stopp := context.WithCancel(context.Background())
	go v.Laufen(ctx)

	if proxyL != nil {
		go func() {
			if err := (&proxy.Proxy{V: v}).Dienen(ctx, proxyL); err != nil {
				log.Printf("Proxy: %v", err)
			}
		}()
	}

	buch := verbrauch.Oeffnen(filepath.Join(ordner, "verbrauch.json"))
	go buchen(ctx, v, buch)

	l := &laden.Lader{V: v, Ordner: func() string { return e.Werte().Ordner }}
	a := &api.API{V: v, L: l, E: e, ProxyAdresse: proxyAdresse, ProxyBereit: proxyL != nil, Version: Version, Beenden: stopp, Buch: buch}
	if err := a.SchluesselSchreiben(); err != nil {
		log.Fatalf("Schlüssel: %v", err)
	}
	// Nur wenn der Proxy-Port wirklich uns gehört, sonst ginge der Verkehr an ein fremdes Programm
	if e.Werte().Systemproxy && proxyL != nil {
		if err := api.SystemproxySchalten(e, proxyAdresse, true); err != nil {
			log.Printf("Systemproxy: %v", err)
		}
	}
	srv := &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(apiL)
	log.Printf("bondrd %s: Steuerung %s, Proxy %s", Version, apiL.Addr(), proxyAdresse)
	if *mitTunnel {
		go tunnelLaufen(ctx, v, stopp)
	}

	if *eltern > 0 {
		go func() {
			for ctx.Err() == nil {
				time.Sleep(2 * time.Second)
				if !lebt(*eltern) {
					log.Printf("App %d beendet, bondrd endet", *eltern)
					stopp()
					return
				}
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
	case <-ctx.Done():
	}
	stopp()
	buch.Speichern()
	// Proxy freigeben, sonst hätten Browser kein Internet; die Einstellung bleibt
	system.ProxyEntfernen(proxyAdresse)
	c, fertig := context.WithTimeout(context.Background(), 2*time.Second)
	defer fertig()
	srv.Shutdown(c)
	os.Remove(api.SchluesselDatei())
}

// buchen überträgt alle paar Sekunden die Zähler des Verteilers ins Verbrauchsbuch und
// speichert es jede Minute.
func buchen(ctx context.Context, v *verteiler.Verteiler, b *verbrauch.Buch) {
	takt := time.NewTicker(5 * time.Second)
	defer takt.Stop()
	for n := 1; ; n++ {
		select {
		case <-ctx.Done():
			return
		case <-takt.C:
			for _, s := range v.Stand() {
				b.Buchen(s.Name, int64(s.MBRein*1e6), int64(s.MBRaus*1e6))
			}
			if n%12 == 0 {
				if err := b.Speichern(); err != nil {
					log.Printf("Verbrauch speichern: %v", err)
				}
			}
		}
	}
}
