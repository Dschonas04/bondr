// Paket wege findet die Netzwerkadapter, über die der Rechner ins Internet kommt,
// und baut Verbindungen, die fest an einen dieser Adapter gebunden sind.
package wege

import (
	"context"
	"net"
	"sort"
	"time"
)

// Weg ist ein Adapter mit eigener IPv4-Adresse und eigenem Gateway.
type Weg struct {
	Schluessel   string `json:"schluessel"` // Name + Adresse, bleibt über Aktualisierungen gleich
	Name         string `json:"name"`
	Beschreibung string `json:"beschreibung"`
	Art          string `json:"art"` // Kabel, WLAN, Handy, Sonstiges
	Index        uint32 `json:"index"`
	IP           net.IP `json:"ip"`
	Gateway      net.IP `json:"gateway"`
}

// Finden liefert alle Adapter, die gerade verbunden sind und ein IPv4-Gateway haben.
func Finden() ([]Weg, error) {
	w, err := finden()
	if err != nil {
		return nil, err
	}
	sort.Slice(w, func(i, j int) bool {
		return rang(w[i].Art) < rang(w[j].Art) || (rang(w[i].Art) == rang(w[j].Art) && w[i].Name < w[j].Name)
	})
	return w, nil
}

func rang(art string) int {
	switch art {
	case "Kabel":
		return 0
	case "WLAN":
		return 1
	case "Handy":
		return 2
	}
	return 3
}

// Waehlen baut eine TCP-Verbindung über genau diesen Weg auf.
func (w Weg) Waehlen(ctx context.Context, adresse string) (net.Conn, error) {
	d := net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		LocalAddr: &net.TCPAddr{IP: w.IP},
		Control:   binden(w.Index),
	}
	return d.DialContext(ctx, "tcp4", adresse)
}

// Aufloesen fragt den Namen ab. Die Antwort kommt vom System; welcher Weg dafür
// benutzt wird, ist egal, die eigentliche Verbindung läuft danach über w.
func (w Weg) Aufloesen(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	return net.DefaultResolver.LookupIP(ctx, "ip4", host)
}

func artAusName(name, beschreibung string) string {
	t := name + " " + beschreibung
	switch {
	case enthaelt(t, "NDIS", "Android", "iPhone", "Apple Mobile", "Samsung", "Mobile", "Tethering"):
		return "Handy"
	case enthaelt(t, "WLAN", "Wi-Fi", "Wireless", "802.11", "wlan", "wlp"):
		return "WLAN"
	case enthaelt(t, "Ethernet", "GbE", "LAN", "eth", "enp", "eno"):
		return "Kabel"
	}
	return "Sonstiges"
}

func enthaelt(s string, teile ...string) bool {
	for _, t := range teile {
		if len(t) <= len(s) {
			for i := 0; i+len(t) <= len(s); i++ {
				if s[i:i+len(t)] == t {
					return true
				}
			}
		}
	}
	return false
}
