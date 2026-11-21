package umleitung

import (
	"encoding/binary"
	"net"
	"reflect"
	"testing"
)

func paket(q [4]byte, qp uint16, z [4]byte, zp uint16, flags byte) []byte {
	p := make([]byte, 40)
	p[0] = 0x45
	p[9] = 6
	copy(p[12:16], q[:])
	copy(p[16:20], z[:])
	binary.BigEndian.PutUint16(p[20:22], qp)
	binary.BigEndian.PutUint16(p[22:24], zp)
	p[33] = flags
	return p
}

func teile(p []byte) (q [4]byte, qp uint16, z [4]byte, zp uint16) {
	copy(q[:], p[12:16])
	copy(z[:], p[16:20])
	return q, binary.BigEndian.Uint16(p[20:22]), z, binary.BigEndian.Uint16(p[22:24])
}

func TestSpiegelnHinUndZurueck(t *testing.T) {
	L := [4]byte{10, 0, 0, 7}
	R := [4]byte{23, 1, 2, 3}
	const E = 47803
	tab := NeueTabelle()
	gefragt := 0
	ja := func(uint16, [4]byte, uint16) bool { gefragt++; return true }

	// Datenpaket ohne SYN zu unbekannter Verbindung bleibt unverändert
	p := paket(L, 50000, R, 80, tcpACK)
	if a := tab.Bearbeiten(p, E, 1, ja); a != Durchlassen || gefragt != 0 {
		t.Fatalf("ohne SYN: %v, gefragt %d", a, gefragt)
	}

	// SYN: L:50000 → R:80 wird R:50000 → L:E
	p = paket(L, 50000, R, 80, tcpSYN)
	if a := tab.Bearbeiten(p, E, 1, ja); a != Spiegeln {
		t.Fatalf("SYN: %v", a)
	}
	if q, qp, z, zp := teile(p); q != R || qp != 50000 || z != L || zp != E {
		t.Fatalf("SYN umgeschrieben zu %v:%d → %v:%d", q, qp, z, zp)
	}

	// Empfänger kennt das ursprüngliche Ziel
	ziel, ok := tab.Ziel(&net.TCPAddr{IP: net.IP(R[:]), Port: 50000})
	if !ok || !ziel.IP.Equal(net.IP(R[:])) || ziel.Port != 80 {
		t.Fatalf("Ziel: %v %v", ziel, ok)
	}
	if _, ok := tab.Ziel(&net.TCPAddr{IP: net.IPv4(9, 9, 9, 9), Port: 50000}); ok {
		t.Fatal("fremde Adresse darf kein Ziel bekommen")
	}

	// Antwort des Empfängers L:E → R:50000 wird R:80 → L:50000
	p = paket(L, E, R, 50000, tcpSYN|tcpACK)
	if a := tab.Bearbeiten(p, E, 2, ja); a != Spiegeln {
		t.Fatalf("Antwort: %v", a)
	}
	if q, qp, z, zp := teile(p); q != R || qp != 80 || z != L || zp != 50000 {
		t.Fatalf("Antwort umgeschrieben zu %v:%d → %v:%d", q, qp, z, zp)
	}

	// weitere Pakete fragen nicht noch einmal
	p = paket(L, 50000, R, 80, tcpACK)
	if a := tab.Bearbeiten(p, E, 3, ja); a != Spiegeln || gefragt != 1 {
		t.Fatalf("Folgepaket: %v, gefragt %d", a, gefragt)
	}
	if tab.Offen() != 1 || tab.Umgeleitet() != 1 {
		t.Fatalf("Offen %d, Umgeleitet %d", tab.Offen(), tab.Umgeleitet())
	}

	tab.Aufraeumen(100, 10)
	if tab.Offen() != 0 {
		t.Fatal("Aufräumen hat nichts entfernt")
	}
}

func TestNichtGewaehltBleibtDirekt(t *testing.T) {
	L := [4]byte{10, 0, 0, 7}
	R := [4]byte{23, 1, 2, 3}
	tab := NeueTabelle()
	nein := func(uint16, [4]byte, uint16) bool { return false }
	p := paket(L, 50001, R, 443, tcpSYN)
	orig := append([]byte(nil), p...)
	if a := tab.Bearbeiten(p, 47803, 1, nein); a != Durchlassen || !reflect.DeepEqual(p, orig) {
		t.Fatalf("nicht gewählt: %v", a)
	}
	// Antwort an unbekannten Port wird verworfen
	if a := tab.Bearbeiten(paket(L, 47803, R, 50001, tcpACK), 47803, 1, nein); a != Verwerfen {
		t.Fatalf("unbekannte Antwort: %v", a)
	}
}

func TestProgramme(t *testing.T) {
	got := Programme(" Steam.exe, EpicGamesLauncher ;C:\\Spiele\\gog.exe, steam.exe,, ")
	want := []string{"steam.exe", "epicgameslauncher.exe", "gog.exe"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
}
