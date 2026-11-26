// Paket verbrauch merkt sich, wie viele Daten über jeden Weg gegangen sind, im
// laufenden Monat und insgesamt, auch über Neustarts hinweg. Wichtig für Mobilfunk,
// wo das Volumen begrenzt ist.
package verbrauch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Zaehler je Weg in Bytes.
type Zaehler struct {
	Monat      string `json:"monat"` // 2026-11
	MonatRein  int64  `json:"monat_rein"`
	MonatRaus  int64  `json:"monat_raus"`
	GesamtRein int64  `json:"gesamt_rein"`
	GesamtRaus int64  `json:"gesamt_raus"`
	SeitDatum  string `json:"seit"` // Datum des letzten Zurücksetzens
}

// Buch hält die Zähler aller Wege, Schlüssel ist der Name des Wegs.
type Buch struct {
	mu      sync.Mutex
	datei   string
	wege    map[string]*Zaehler
	zuletzt map[string][2]int64 // letzter gesehener Stand je Weg (rein, raus)
	jetzt   func() time.Time
}

// Oeffnen liest die Datei oder fängt leer an.
func Oeffnen(datei string) *Buch {
	b := &Buch{datei: datei, wege: map[string]*Zaehler{}, zuletzt: map[string][2]int64{}, jetzt: time.Now}
	if d, err := os.ReadFile(datei); err == nil {
		json.Unmarshal(d, &b.wege)
	}
	if b.wege == nil {
		b.wege = map[string]*Zaehler{}
	}
	return b
}

func (b *Buch) monat() string { return b.jetzt().Format("2006-01") }

// Buchen nimmt den aktuellen Gesamtstand eines Wegs seit Programmstart (Bytes) und
// rechnet den Zuwachs seit dem letzten Aufruf an.
func (b *Buch) Buchen(name string, rein, raus int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	alt, gesehen := b.zuletzt[name]
	b.zuletzt[name] = [2]int64{rein, raus}
	if !gesehen || rein < alt[0] || raus < alt[1] {
		// erster Stand nach dem Start oder der Weg ist neu aufgetaucht: Zähler ab null
		alt = [2]int64{0, 0}
	}
	dr, du := rein-alt[0], raus-alt[1]
	z := b.zaehler(name)
	z.MonatRein += dr
	z.MonatRaus += du
	z.GesamtRein += dr
	z.GesamtRaus += du
}

func (b *Buch) zaehler(name string) *Zaehler {
	z := b.wege[name]
	m := b.monat()
	if z == nil {
		z = &Zaehler{Monat: m, SeitDatum: b.jetzt().Format("2006-01-02")}
		b.wege[name] = z
	}
	if z.Monat != m {
		z.Monat, z.MonatRein, z.MonatRaus = m, 0, 0
	}
	return z
}

// Stand liefert eine Kopie aller Zähler.
func (b *Buch) Stand() map[string]Zaehler {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string]Zaehler, len(b.wege))
	for k := range b.wege {
		out[k] = *b.zaehler(k)
	}
	return out
}

// Zuruecksetzen setzt einen Weg (oder alle, name leer) auf null.
func (b *Buch) Zuruecksetzen(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	heute := b.jetzt().Format("2006-01-02")
	for k := range b.wege {
		if name == "" || k == name {
			b.wege[k] = &Zaehler{Monat: b.monat(), SeitDatum: heute}
		}
	}
}

// Speichern schreibt die Zähler in die Datei.
func (b *Buch) Speichern() error {
	b.mu.Lock()
	d, err := json.MarshalIndent(b.wege, "", "  ")
	b.mu.Unlock()
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(b.datei), 0o700)
	tmp := b.datei + ".neu"
	if err := os.WriteFile(tmp, d, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, b.datei)
}
