// Paket umleitung leitet Verbindungen ausgewählter Programme (etwa Steam) ohne Proxy-Einstellung
// über Bondr. Unter Windows fängt WinDivert die Pakete ab und spiegelt sie an einen lokalen
// Empfänger, der jede Verbindung über den SOCKS-Proxy von bondrd weitergibt. So verteilen sich
// auch Programme, die den Windows-Proxy nicht beachten, auf alle Wege.
//
// Spiegeln: aus L:p → R:q (Programm zum Server) wird R:p → L:E (an den Empfänger E), aus der
// Antwort L:E → R:p wird wieder R:q → L:p. Der Schlüssel ist der lokale Port p des Programms.
package umleitung

import (
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
)

// Aktion sagt, was mit einem abgefangenen Paket geschehen soll.
type Aktion int

const (
	Durchlassen Aktion = iota // unverändert weiterschicken
	Spiegeln                  // umgeschrieben als eingehendes Paket zurückgeben
	Verwerfen                 // nicht weiterschicken
)

const (
	tcpFIN = 0x01
	tcpSYN = 0x02
	tcpRST = 0x04
	tcpACK = 0x10
)

type eintrag struct {
	ziel     [4]byte // R
	zielPort uint16  // q
	zuletzt  atomic.Int64
}

// Tabelle merkt sich, welche Verbindungen umgeleitet werden.
type Tabelle struct {
	mu         sync.RWMutex
	m          map[uint16]*eintrag // lokaler Port des Programms
	umgeleitet atomic.Int64
}

// NeueTabelle legt eine leere Tabelle an.
func NeueTabelle() *Tabelle { return &Tabelle{m: map[uint16]*eintrag{}} }

// Bearbeiten prüft ein ausgehendes IPv4-Paket und schreibt es zum Spiegeln um. empfaenger ist
// der Port des lokalen Empfängers. entscheiden wird nur für das erste SYN einer Verbindung
// gefragt, ob sie umgeleitet werden soll.
func (t *Tabelle) Bearbeiten(p []byte, empfaenger uint16, jetzt int64, entscheiden func(lokalPort uint16, ziel [4]byte, zielPort uint16) bool) Aktion {
	if len(p) < 20 || p[0]>>4 != 4 || p[9] != 6 {
		return Durchlassen
	}
	ihl := int(p[0]&0x0f) * 4
	if ihl < 20 || len(p) < ihl+20 {
		return Durchlassen
	}
	tcp := p[ihl:]
	quellPort := binary.BigEndian.Uint16(tcp[0:2])
	zielPort := binary.BigEndian.Uint16(tcp[2:4])
	flags := tcp[13]
	var quelle, ziel [4]byte
	copy(quelle[:], p[12:16])
	copy(ziel[:], p[16:20])

	if quellPort == empfaenger {
		// Antwort des Empfängers an R:p: zurück an das Programm als R:q → L:p
		t.mu.RLock()
		e := t.m[zielPort]
		t.mu.RUnlock()
		if e == nil || e.ziel != ziel {
			return Verwerfen
		}
		e.zuletzt.Store(jetzt)
		copy(p[12:16], ziel[:])
		copy(p[16:20], quelle[:])
		binary.BigEndian.PutUint16(tcp[0:2], e.zielPort)
		return Spiegeln
	}

	t.mu.RLock()
	e := t.m[quellPort]
	t.mu.RUnlock()
	passt := e != nil && e.ziel == ziel && e.zielPort == zielPort
	if !passt {
		if flags&tcpSYN == 0 || flags&tcpACK != 0 {
			return Durchlassen
		}
		if !entscheiden(quellPort, ziel, zielPort) {
			if e != nil { // alter Eintrag mit demselben Port gehört zu einer beendeten Verbindung
				t.mu.Lock()
				if t.m[quellPort] == e {
					delete(t.m, quellPort)
				}
				t.mu.Unlock()
			}
			return Durchlassen
		}
		e = &eintrag{ziel: ziel, zielPort: zielPort}
		t.mu.Lock()
		t.m[quellPort] = e
		t.mu.Unlock()
		t.umgeleitet.Add(1)
	}
	e.zuletzt.Store(jetzt)
	// L:p → R:q wird R:p → L:E
	copy(p[12:16], ziel[:])
	copy(p[16:20], quelle[:])
	binary.BigEndian.PutUint16(tcp[2:4], empfaenger)
	return Spiegeln
}

// Ziel liefert das ursprüngliche Ziel für eine beim Empfänger angenommene Verbindung von
// gegenueber (R:p).
func (t *Tabelle) Ziel(gegenueber *net.TCPAddr) (*net.TCPAddr, bool) {
	ip := gegenueber.IP.To4()
	if ip == nil || gegenueber.Port <= 0 || gegenueber.Port > 65535 {
		return nil, false
	}
	t.mu.RLock()
	e := t.m[uint16(gegenueber.Port)]
	t.mu.RUnlock()
	if e == nil || !ip.Equal(net.IP(e.ziel[:])) {
		return nil, false
	}
	return &net.TCPAddr{IP: net.IP(append([]byte(nil), e.ziel[:]...)), Port: int(e.zielPort)}, true
}

// Aufraeumen entfernt Einträge, die länger als ruhe (gleiche Einheit wie jetzt) nichts
// geschickt haben.
func (t *Tabelle) Aufraeumen(jetzt, ruhe int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, e := range t.m {
		if jetzt-e.zuletzt.Load() > ruhe {
			delete(t.m, k)
		}
	}
}

// Offen ist die Zahl der gemerkten Verbindungen.
func (t *Tabelle) Offen() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.m)
}

// Umgeleitet zählt alle bisher umgeleiteten Verbindungen.
func (t *Tabelle) Umgeleitet() int64 { return t.umgeleitet.Load() }
