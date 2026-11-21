// Paket einstellungen speichert, was der Nutzer eingestellt hat.
package einstellungen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Rollen eines Wegs, angelehnt an nginx-Upstreams.
const (
	RolleHaupt   = "haupt"   // wird nach Gewicht benutzt
	RolleReserve = "reserve" // nur, wenn kein Hauptweg geht (nginx: backup)
	RolleAus     = "aus"     // nie
)

// Strategien für die Verteilung neuer Verbindungen.
const (
	StrategieWenigste = "wenigste" // gewichtet, wenigste offene Verbindungen (nginx: least_conn)
	StrategieReihum   = "reihum"   // gewichtet reihum (nginx: Standard)
	StrategieAusfall  = "ausfall"  // nur der stärkste Hauptweg, die anderen bei Ausfall
)

// Regel gilt für einen Adapter.
type Regel struct {
	Gewicht int    `json:"gewicht"` // 1 bis 10
	Rolle   string `json:"rolle"`
}

// Werte sind alle Einstellungen.
type Werte struct {
	Systemproxy bool             `json:"systemproxy"`
	Strategie   string           `json:"strategie"`
	Regeln      map[string]Regel `json:"regeln"` // Schlüssel: Adaptername
	Ordner      string           `json:"ordner"`
}

// Vorgabe liefert die Regel für einen Adapter, für den nichts eingestellt ist: Kabel und
// WLAN zum Router werden bevorzugt, ein Handy am USB-Kabel läuft mit wenig Gewicht mit.
func Vorgabe(art string) Regel {
	switch art {
	case "Kabel":
		return Regel{Gewicht: 10, Rolle: RolleHaupt}
	case "WLAN":
		return Regel{Gewicht: 7, Rolle: RolleHaupt}
	case "Handy":
		return Regel{Gewicht: 3, Rolle: RolleHaupt}
	}
	return Regel{Gewicht: 3, Rolle: RolleHaupt}
}

// Speicher liest und schreibt die Datei.
type Speicher struct {
	mu    sync.Mutex
	datei string
	w     Werte
}

// Ordner ist %APPDATA%\Bondr bzw. ~/Library/Application Support/Bondr.
func Ordner() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = "."
	}
	return filepath.Join(d, "Bondr")
}

// Laden öffnet die Einstellungen oder legt Vorgaben an.
func Laden() *Speicher {
	s := &Speicher{datei: filepath.Join(Ordner(), "einstellungen.json")}
	if b, err := os.ReadFile(s.datei); err == nil {
		json.Unmarshal(b, &s.w)
	}
	if s.w.Ordner == "" {
		if h, err := os.UserHomeDir(); err == nil {
			s.w.Ordner = filepath.Join(h, "Downloads")
		}
	}
	if s.w.Strategie == "" {
		s.w.Strategie = StrategieWenigste
	}
	if s.w.Regeln == nil {
		s.w.Regeln = map[string]Regel{}
	}
	return s
}

// Werte liefert eine Kopie.
func (s *Speicher) Werte() Werte {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.w
	w.Regeln = make(map[string]Regel, len(s.w.Regeln))
	for k, v := range s.w.Regeln {
		w.Regeln[k] = v
	}
	return w
}

// Regel liefert die Regel für einen Adapter, sonst die Vorgabe für seine Art.
func (s *Speicher) Regel(name, art string) Regel {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.w.Regeln[name]; ok {
		return r
	}
	return Vorgabe(art)
}

// Strategie liefert die eingestellte Strategie.
func (s *Speicher) Strategie() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Strategie
}

// Aendern wendet f an und speichert.
func (s *Speicher) Aendern(f func(*Werte)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.w)
	if err := os.MkdirAll(filepath.Dir(s.datei), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s.w, "", "  ")
	tmp := s.datei + ".neu"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.datei)
}
