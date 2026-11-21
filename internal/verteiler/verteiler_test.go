package verteiler

import (
	"net"
	"testing"

	"github.com/Dschonas04/bondr/internal/einstellungen"
	"github.com/Dschonas04/bondr/internal/wege"
)

type regeln struct {
	r map[string]einstellungen.Regel
	s string
}

func (x regeln) Regel(name, art string) einstellungen.Regel {
	if r, ok := x.r[name]; ok {
		return r
	}
	return einstellungen.Vorgabe(art)
}
func (x regeln) Strategie() string { return x.s }

func drei() ([]wege.Weg, error) {
	ip := net.IPv4(127, 0, 0, 1)
	return []wege.Weg{
		{Schluessel: "k", Name: "Ethernet", Art: "Kabel", IP: ip},
		{Schluessel: "w", Name: "WLAN", Art: "WLAN", IP: ip},
		{Schluessel: "h", Name: "Handy", Art: "Handy", IP: ip},
	}, nil
}

func namen(l []*Pfad) []string {
	var n []string
	for _, p := range l {
		n = append(n, p.Name)
	}
	return n
}

func TestReihumNachGewicht(t *testing.T) {
	v := NeuMit(drei, regeln{s: einstellungen.StrategieReihum})
	zahl := map[string]int{}
	for i := 0; i < 200; i++ {
		zahl[v.pfadeOrdnen(true)[0].Name]++
	}
	// Gewichte 10 : 7 : 3 von 20
	if zahl["Ethernet"] != 100 || zahl["WLAN"] != 70 || zahl["Handy"] != 30 {
		t.Fatalf("Verteilung %v", zahl)
	}
}

func TestWenigsteBevorzugtGewicht(t *testing.T) {
	v := NeuMit(drei, regeln{s: einstellungen.StrategieWenigste})
	zahl := map[string]int{}
	for i := 0; i < 20; i++ {
		p := v.pfadeOrdnen(true)[0]
		p.offen.Add(1) // Verbindung bleibt offen
		zahl[p.Name]++
	}
	if zahl["Ethernet"] != 10 || zahl["WLAN"] != 7 || zahl["Handy"] != 3 {
		t.Fatalf("Verteilung %v", zahl)
	}
}

func TestReserveNurBeiAusfall(t *testing.T) {
	r := regeln{s: einstellungen.StrategieWenigste, r: map[string]einstellungen.Regel{
		"Handy": {Gewicht: 5, Rolle: einstellungen.RolleReserve},
		"WLAN":  {Gewicht: 5, Rolle: einstellungen.RolleAus},
	}}
	v := NeuMit(drei, r)
	if n := namen(v.Pfade()); len(n) != 2 || n[0] != "Ethernet" || n[1] != "Handy" {
		t.Fatalf("Reihenfolge %v", n)
	}
	l, anz := v.LadePfade(4)
	if len(l) != 1 || l[0].Name != "Ethernet" || anz[0] != 4 {
		t.Fatalf("Download ohne Ausfall: %v %v", namen(l), anz)
	}
	// Ethernet fällt aus
	for _, p := range v.Pfade() {
		if p.Name == "Ethernet" {
			p.stoeren(1e10, nil)
		}
	}
	if n := namen(v.Pfade()); n[0] != "Handy" {
		t.Fatalf("nach Ausfall %v", n)
	}
	l, _ = v.LadePfade(4)
	if len(l) != 1 || l[0].Name != "Handy" {
		t.Fatalf("Download nach Ausfall: %v", namen(l))
	}
}

func TestAusfallNurStaerkster(t *testing.T) {
	v := NeuMit(drei, regeln{s: einstellungen.StrategieAusfall})
	for i := 0; i < 5; i++ {
		if p := v.pfadeOrdnen(true)[0]; p.Name != "Ethernet" {
			t.Fatalf("erwartet Ethernet, bekam %s", p.Name)
		}
	}
	l, _ := v.LadePfade(4)
	if len(l) != 1 {
		t.Fatalf("ausfall lädt über %v", namen(l))
	}
}

func TestLadeAnteile(t *testing.T) {
	v := NeuMit(drei, nil)
	l, anz := v.LadePfade(4)
	got := map[string]int{}
	for i, p := range l {
		got[p.Name] = anz[i]
	}
	if got["Ethernet"] != 4 || got["WLAN"] != 3 || got["Handy"] != 1 {
		t.Fatalf("Anteile %v", got)
	}
}
