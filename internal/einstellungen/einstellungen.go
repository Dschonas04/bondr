// Paket einstellungen speichert, was der Nutzer eingestellt hat.
package einstellungen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Werte sind alle Einstellungen.
type Werte struct {
	Systemproxy bool     `json:"systemproxy"`
	Aus         []string `json:"aus"`    // abgeschaltete Adapter
	Ordner      string   `json:"ordner"` // Ziel für Downloads
}

// Speicher liest und schreibt die Datei.
type Speicher struct {
	mu    sync.Mutex
	datei string
	w     Werte
}

// Ordner ist %APPDATA%\Bondr (bzw. ~/.config/bondr).
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
	return s
}

// Werte liefert eine Kopie.
func (s *Speicher) Werte() Werte {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.w
	w.Aus = append([]string(nil), s.w.Aus...)
	return w
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
