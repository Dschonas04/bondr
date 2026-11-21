package laden

import (
	"bytes"
	"crypto/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dschonas04/bondr/internal/verteiler"
	"github.com/Dschonas04/bondr/internal/wege"
)

func zweiWege() ([]wege.Weg, error) {
	return []wege.Weg{
		{Schluessel: "a", Name: "Kabel", Art: "Kabel", IP: net.IPv4(127, 0, 0, 1)},
		{Schluessel: "b", Name: "Handy", Art: "Handy", IP: net.IPv4(127, 0, 0, 1)},
	}, nil
}

func TestStueckeUeberAlleWege(t *testing.T) {
	inhalt := make([]byte, 37<<20+123)
	rand.Read(inhalt)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "test.bin", time.Now(), bytes.NewReader(inhalt))
	}))
	defer srv.Close()

	ordner := t.TempDir()
	l := &Lader{V: verteiler.NeuMit(zweiWege, nil), Ordner: func() string { return ordner }}
	a, err := l.Starten(srv.URL + "/datei/test.bin")
	if err != nil {
		t.Fatal(err)
	}
	ende := time.Now().Add(30 * time.Second)
	for time.Now().Before(ende) {
		e := l.Liste()[0]
		if e.Status != "läuft" {
			if e.Status != "fertig" {
				t.Fatalf("Status %s: %s", e.Status, e.Fehler)
			}
			got, err := os.ReadFile(filepath.Join(ordner, "test.bin"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, inhalt) {
				t.Fatal("Inhalt weicht ab")
			}
			if len(e.Wege) != 2 {
				t.Fatalf("erwartet zwei Wege, bekam %v", e.Wege)
			}
			var summe int64
			for _, w := range e.Wege {
				if w.Bytes == 0 {
					t.Fatalf("Weg %s hat nichts geladen", w.Name)
				}
				summe += w.Bytes
			}
			if summe != int64(len(inhalt)) || e.Geladen != int64(len(inhalt)) {
				t.Fatalf("Summe %d, geladen %d, erwartet %d", summe, e.Geladen, len(inhalt))
			}
			_ = a
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("Zeit abgelaufen")
}

func TestOhneRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="bericht.txt"`)
		w.Write([]byte("hallo"))
	}))
	defer srv.Close()
	ordner := t.TempDir()
	l := &Lader{V: verteiler.NeuMit(zweiWege, nil), Ordner: func() string { return ordner }}
	l.Starten(srv.URL)
	for i := 0; i < 200 && l.Liste()[0].Status == "läuft"; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if e := l.Liste()[0]; e.Status != "fertig" {
		t.Fatalf("%s %s", e.Status, e.Fehler)
	}
	b, _ := os.ReadFile(filepath.Join(ordner, "bericht.txt"))
	if string(b) != "hallo" {
		t.Fatalf("Inhalt %q", b)
	}
}

func TestSauber(t *testing.T) {
	for ein, aus := range map[string]string{`..\..\boot.ini`: "boot.ini", "a:b?.zip": "a_b_.zip", "...": "download.bin"} {
		if g := sauber(ein); g != aus {
			t.Errorf("%q → %q, erwartet %q", ein, g, aus)
		}
	}
}

func TestBremseBeiZuVielen(t *testing.T) {
	inhalt := make([]byte, 20<<20)
	rand.Read(inhalt)
	var laufend atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if laufend.Add(1) > 3 {
			laufend.Add(-1)
			w.Header().Set("Retry-After", "1")
			http.Error(w, "zu viele", http.StatusTooManyRequests)
			return
		}
		defer laufend.Add(-1)
		http.ServeContent(w, r, "b.bin", time.Now(), bytes.NewReader(inhalt))
	}))
	defer srv.Close()
	ordner := t.TempDir()
	l := &Lader{V: verteiler.NeuMit(zweiWege, nil), Ordner: func() string { return ordner }}
	l.Starten(srv.URL + "/b.bin")
	for i := 0; i < 600 && l.Liste()[0].Status == "läuft"; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if e := l.Liste()[0]; e.Status != "fertig" {
		t.Fatalf("%s %s", e.Status, e.Fehler)
	}
	got, _ := os.ReadFile(filepath.Join(ordner, "b.bin"))
	if !bytes.Equal(got, inhalt) {
		t.Fatal("Inhalt weicht ab")
	}
}
