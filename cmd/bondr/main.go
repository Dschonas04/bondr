// Bondr verteilt Verbindungen auf alle Internetzugänge des Rechners (Kabel, WLAN,
// Handy per USB) und lädt Dateien in Stücken über alle gleichzeitig.
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
	"strings"
	"syscall"
	"time"

	"github.com/Dschonas04/bondr/internal/einstellungen"
	"github.com/Dschonas04/bondr/internal/laden"
	"github.com/Dschonas04/bondr/internal/oberflaeche"
	"github.com/Dschonas04/bondr/internal/proxy"
	"github.com/Dschonas04/bondr/internal/system"
	"github.com/Dschonas04/bondr/internal/verteiler"
)

// Version wird beim Bau gesetzt.
var Version = "dev"

func main() {
	still := flag.Bool("still", false, "ohne Browserfenster starten (Autostart)")
	uiPort := flag.Int("ui", 8765, "Port der Oberfläche")
	proxyPort := flag.Int("proxy", 1080, "Port des Proxys (SOCKS5 und HTTP)")
	flag.Parse()

	ordner := einstellungen.Ordner()
	os.MkdirAll(ordner, 0o755)
	if f, err := os.OpenFile(filepath.Join(ordner, "bondr.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(f)
	}
	schluesselDatei := filepath.Join(ordner, "sitzung")

	uiAdresse := fmt.Sprintf("127.0.0.1:%d", *uiPort)
	uiL, err := net.Listen("tcp", uiAdresse)
	if err != nil {
		// Läuft schon: nur das Fenster der laufenden Instanz öffnen.
		if b, e := os.ReadFile(schluesselDatei); e == nil && !*still {
			system.Oeffnen("http://" + uiAdresse + "/#k=" + strings.TrimSpace(string(b)))
		}
		return
	}
	proxyAdresse := fmt.Sprintf("127.0.0.1:%d", *proxyPort)
	proxyL, err := net.Listen("tcp", proxyAdresse)
	if err != nil {
		log.Fatalf("Proxy-Port %s belegt: %v", proxyAdresse, err)
	}

	e := einstellungen.Laden()
	v := verteiler.Neu(e.Werte().Aus, func(aus []string) {
		e.Aendern(func(w *einstellungen.Werte) { w.Aus = aus })
	})
	ctx, stopp := context.WithCancel(context.Background())
	go v.Laufen(ctx)

	p := &proxy.Proxy{V: v}
	go func() {
		if err := p.Dienen(ctx, proxyL); err != nil {
			log.Printf("Proxy: %v", err)
		}
	}()

	l := &laden.Lader{V: v, Ordner: func() string { return e.Werte().Ordner }}
	o := &oberflaeche.Oberflaeche{V: v, L: l, E: e, ProxyAdresse: proxyAdresse, Version: Version, Beenden: stopp}
	os.WriteFile(schluesselDatei, []byte(o.Schluessel()), 0o600)

	if e.Werte().Systemproxy {
		if err := oberflaeche.SystemproxySchalten(e, proxyAdresse, true); err != nil {
			log.Printf("Systemproxy: %v", err)
		}
	}

	srv := &http.Server{Handler: o.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(uiL)
	log.Printf("Bondr %s: Oberfläche %s, Proxy %s", Version, uiAdresse, proxyAdresse)
	if !*still {
		system.Oeffnen("http://" + uiAdresse + "/#k=" + o.Schluessel())
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
	case <-ctx.Done():
	}
	stopp()
	// Beim Beenden den Proxy freigeben, sonst hätten Browser kein Internet. Die
	// Einstellung bleibt, beim nächsten Start schaltet Bondr ihn wieder ein.
	system.ProxyEntfernen(proxyAdresse)
	c, fertig := context.WithTimeout(context.Background(), 2*time.Second)
	defer fertig()
	srv.Shutdown(c)
	os.Remove(schluesselDatei)
}
