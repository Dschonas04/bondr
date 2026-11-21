package umleitung

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Dschonas04/bondr/internal/einstellungen"
)

// Aufgabe ist der Name der geplanten Aufgabe, die den Umleiter mit Adminrechten startet.
const Aufgabe = "Bondr Umleitung"

// EmpfaengerPort ist der Port, auf dem der Umleiter gespiegelte Verbindungen annimmt.
const EmpfaengerPort = 47803

// Status schreibt der Umleiter jede Sekunde, die App zeigt ihn an.
type Status struct {
	Zeit       int64  `json:"zeit"`
	Aktiv      bool   `json:"aktiv"`
	Umgeleitet int64  `json:"umgeleitet"`
	Offen      int    `json:"offen"`
	Fehler     string `json:"fehler,omitempty"`
}

// StatusDatei liegt neben den Einstellungen.
func StatusDatei() string { return filepath.Join(einstellungen.Ordner(), "umleitung.json") }

func statusSchreiben(s Status) {
	s.Zeit = time.Now().Unix()
	b, _ := json.Marshal(s)
	tmp := StatusDatei() + ".neu"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, StatusDatei())
	}
}

// Stand fasst Einstellung und Status für die Steuerung zusammen.
func Stand(w einstellungen.Werte) map[string]any {
	var s Status
	if b, err := os.ReadFile(StatusDatei()); err == nil {
		json.Unmarshal(b, &s)
	}
	frisch := time.Since(time.Unix(s.Zeit, 0)) < 5*time.Second
	laeuft := frisch && s.Fehler == ""
	fehler := s.Fehler
	if time.Since(time.Unix(s.Zeit, 0)) > 2*time.Minute {
		fehler = ""
	}
	return map[string]any{
		"gewuenscht": w.Umleitung,
		"programme":  strings.Join(w.Programme, ", "),
		"laeuft":     laeuft,
		"aktiv":      laeuft && s.Aktiv,
		"umgeleitet": s.Umgeleitet,
		"offen":      s.Offen,
		"fehler":     fehler,
	}
}

// Programme zerlegt eine Eingabe wie "steam.exe, EpicGamesLauncher" in Dateinamen.
func Programme(eingabe string) []string {
	var aus []string
	gesehen := map[string]bool{}
	for _, t := range strings.FieldsFunc(eingabe, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		t = strings.TrimSpace(t)
		if i := strings.LastIndexAny(t, `\/`); i >= 0 {
			t = t[i+1:]
		}
		t = strings.ToLower(t)
		if t == "" || t == "." {
			continue
		}
		if !strings.HasSuffix(t, ".exe") {
			t += ".exe"
		}
		if !gesehen[t] {
			gesehen[t] = true
			aus = append(aus, t)
		}
	}
	return aus
}
