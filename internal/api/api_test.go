package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dschonas04/bondr/internal/einstellungen"
)

func TestSpeichernMeldetFehler(t *testing.T) {
	d := t.TempDir()
	for _, v := range []string{"XDG_CONFIG_HOME", "HOME", "APPDATA"} {
		t.Setenv(v, d)
	}
	a := &API{E: einstellungen.Laden(), schluessel: "test"}
	ruf := func(pfad, inhalt string) int {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+pfad, strings.NewReader(inhalt))
		r.Header.Set("X-Bondr", "test")
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if c := ruf("/ordner", `{"ordner":" "}`); c != http.StatusBadRequest {
		t.Errorf("leerer Ordner: %d", c)
	}
	// Eine Datei, wo der Ordner sein müsste: Speichern kann nicht klappen.
	if err := os.MkdirAll(filepath.Dir(einstellungen.Ordner()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(einstellungen.Ordner(), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for pfad, inhalt := range map[string]string{
		"/regel":     `{"name":"eth0","gewicht":5,"rolle":"haupt"}`,
		"/strategie": `{"strategie":"reihum"}`,
		"/ordner":    `{"ordner":"/tmp"}`,
		"/umleitung": `{"an":true}`,
	} {
		if c := ruf(pfad, inhalt); c != http.StatusInternalServerError {
			t.Errorf("%s meldet %d, obwohl nichts gespeichert ist", pfad, c)
		}
	}
	if a.E.Strategie() != einstellungen.StrategieWenigste {
		t.Error("Nicht Gespeichertes gilt trotzdem")
	}
}
