package verbrauch

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBuchen(t *testing.T) {
	d := filepath.Join(t.TempDir(), "v.json")
	jetzt := time.Date(2026, 11, 30, 12, 0, 0, 0, time.UTC)
	b := Oeffnen(d)
	b.jetzt = func() time.Time { return jetzt }
	b.Buchen("Mobilfunk", 1000, 100)
	b.Buchen("Mobilfunk", 3000, 300)
	if z := b.Stand()["Mobilfunk"]; z.MonatRein != 3000 || z.GesamtRaus != 300 {
		t.Fatalf("falsch gebucht: %+v", z)
	}
	if err := b.Speichern(); err != nil {
		t.Fatal(err)
	}
	// Neustart: Zähler des Kerns fangen bei null an, die Summe bleibt
	b2 := Oeffnen(d)
	b2.jetzt = func() time.Time { return jetzt }
	b2.Buchen("Mobilfunk", 500, 0)
	if z := b2.Stand()["Mobilfunk"]; z.GesamtRein != 3500 {
		t.Fatalf("nach Neustart %+v", z)
	}
	// neuer Monat: Monatszähler auf null, gesamt bleibt
	jetzt = time.Date(2026, 12, 1, 0, 0, 1, 0, time.UTC)
	b2.Buchen("Mobilfunk", 700, 0)
	if z := b2.Stand()["Mobilfunk"]; z.MonatRein != 200 || z.GesamtRein != 3700 || z.Monat != "2026-12" {
		t.Fatalf("Monatswechsel %+v", z)
	}
	b2.Zuruecksetzen("")
	if z := b2.Stand()["Mobilfunk"]; z.GesamtRein != 0 {
		t.Fatalf("zurückgesetzt %+v", z)
	}
}
