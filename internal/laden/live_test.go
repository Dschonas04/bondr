//go:build live

package laden

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/Dschonas04/bondr/internal/verteiler"
	"github.com/Dschonas04/bondr/internal/wege"
)

func TestLive(t *testing.T) {
	log.SetOutput(os.Stderr)
	ordner := t.TempDir()
	l := &Lader{V: verteiler.NeuMit(wege.Finden, nil), Ordner: func() string { return ordner }}
	l.Starten(os.Getenv("URL"))
	t0 := time.Now()
	for i := 0; i < 60; i++ {
		time.Sleep(time.Second)
		e := l.Liste()[0]
		t.Logf("%2ds %5.0f MB %4.0f Mbit/s %s", int(time.Since(t0).Seconds()), float64(e.Geladen)/1e6, e.Mbit, e.Status)
		if e.Status != "läuft" {
			t.Log(e.Fehler)
			return
		}
	}
}
