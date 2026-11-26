// Paket wege findet die Netzwerkadapter, über die der Rechner ins Internet kommt,
// und baut Verbindungen, die fest an einen dieser Adapter gebunden sind.
package wege

import (
	"context"
	"net"
	"sort"
	"sync"
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
	// DNS sind die Namensserver dieses Netzes; nur unter Android gesetzt, wo jedes Netz
	// seine eigenen hat und Bondr selbst auflöst.
	DNS []net.IP `json:"dns,omitempty"`
	// IPv6 sagt, ob das Netz selbst IPv6 kann (Android meldet das).
	IPv6 bool `json:"ipv6,omitempty"`
}

var extern struct {
	sync.Mutex
	wege []Weg
}

// Setzen übernimmt die Wege, die das Betriebssystem der App meldet (Android: die App
// kennt die Netze über den ConnectivityManager, der Kern selbst nicht).
func Setzen(l []Weg) {
	extern.Lock()
	defer extern.Unlock()
	extern.wege = append([]Weg(nil), l...)
}

func gesetzt() []Weg {
	extern.Lock()
	defer extern.Unlock()
	return append([]Weg(nil), extern.wege...)
}

// Finden liefert alle Adapter, die gerade verbunden sind und ein IPv4-Gateway haben.
func Finden() ([]Weg, error) {
	w, err := finden()
	if err != nil {
		return nil, err
	}
	sort.Slice(w, func(i, j int) bool {
		return Rang(w[i].Art) < Rang(w[j].Art) || (Rang(w[i].Art) == Rang(w[j].Art) && w[i].Name < w[j].Name)
	})
	return w, nil
}

// Rang ordnet Arten für die Anzeige: Kabel, WLAN, Handy, Rest.
func Rang(art string) int {
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
		Control:   binden(w.Index),
	}
	netz := "tcp4"
	if h, _, err := net.SplitHostPort(adresse); err == nil {
		if ip := net.ParseIP(h); ip != nil && ip.To4() == nil {
			netz = "tcp6" // IPv6-Ziel: das Netz wählt die eigene Adresse
		}
	}
	// Ohne eigene IPv4-Adresse (Mobilfunk nur mit IPv6 und 464XLAT) wählt das Netz sie selbst
	if netz == "tcp4" && !w.IP.IsUnspecified() {
		d.LocalAddr = &net.TCPAddr{IP: w.IP}
	}
	return d.DialContext(ctx, netz, adresse)
}

// Aufloesen fragt den Namen ab. Die Antwort kommt vom System; welcher Weg dafür
// benutzt wird, ist egal, die eigentliche Verbindung läuft danach über w.
func (w Weg) Aufloesen(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	return aufloeser(w).LookupIP(ctx, "ip4", host)
}

// DNS4 liefert den ersten IPv4-Namensserver des Wegs.
func (w Weg) DNS4() net.IP {
	for _, d := range w.DNS {
		if d.To4() != nil {
			return d.To4()
		}
	}
	return nil
}

// Paket öffnet einen UDP-Socket, der fest an diesen Weg gebunden ist.
func (w Weg) Paket(ctx context.Context, v6 bool) (net.PacketConn, error) {
	lc := net.ListenConfig{Control: binden(w.Index)}
	if v6 {
		return lc.ListenPacket(ctx, "udp6", "[::]:0")
	}
	ip := w.IP
	if ip == nil {
		ip = net.IPv4zero
	}
	return lc.ListenPacket(ctx, "udp4", net.JoinHostPort(ip.String(), "0"))
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
