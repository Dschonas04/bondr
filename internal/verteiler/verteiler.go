// Paket verteiler entscheidet für jede neue Verbindung, über welchen Weg sie läuft,
// und zählt, wie viel über jeden Weg geht.
package verteiler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Dschonas04/bondr/internal/wege"
)

// Pfad ist ein Weg mit seinem Laufzeitzustand.
type Pfad struct {
	wege.Weg
	offen      atomic.Int64
	zuletzt    atomic.Int64 // wann der Weg zuletzt eine Verbindung bekam
	rein       atomic.Int64 // Bytes insgesamt
	raus       atomic.Int64
	gestoert   atomic.Int64 // Unix-Nanosekunden, bis wann der Weg gemieden wird
	letzteRein int64
	letzteRaus int64
	rateRein   float64 // Bytes/s
	rateRaus   float64
	spitze     float64 // höchste Empfangsrate der letzten Zeit, Bytes/s
	fehler     string
}

// Stand ist, was die Oberfläche je Weg anzeigt.
type Stand struct {
	wege.Weg
	Aktiv      bool    `json:"aktiv"`
	Gestoert   bool    `json:"gestoert"`
	Fehler     string  `json:"fehler,omitempty"`
	Offen      int64   `json:"offen"`
	MbitRein   float64 `json:"mbit_rein"`
	MbitRaus   float64 `json:"mbit_raus"`
	MbitSpitze float64 `json:"mbit_spitze"`
	GBRein     float64 `json:"gb_rein"`
	GBRaus     float64 `json:"gb_raus"`
}

// Verteiler hält alle bekannten Wege.
type Verteiler struct {
	mu           sync.Mutex
	pfade        map[string]*Pfad
	aus          map[string]bool // vom Nutzer abgeschaltet, Schlüssel = Adaptername
	finden       func() ([]wege.Weg, error)
	beiAenderung func(aus []string)
}

// Neu legt einen Verteiler an. aus sind Adapternamen, die nicht benutzt werden sollen.
func Neu(aus []string, beiAenderung func([]string)) *Verteiler {
	v := &Verteiler{pfade: map[string]*Pfad{}, aus: map[string]bool{}, finden: wege.Finden, beiAenderung: beiAenderung}
	for _, a := range aus {
		v.aus[a] = true
	}
	v.Aktualisieren()
	return v
}

// NeuMit nimmt eine eigene Suchfunktion, zum Testen.
func NeuMit(finden func() ([]wege.Weg, error)) *Verteiler {
	v := &Verteiler{pfade: map[string]*Pfad{}, aus: map[string]bool{}, finden: finden}
	v.Aktualisieren()
	return v
}

// Laufen hält die Wegliste aktuell und misst einmal pro Sekunde die Raten.
func (v *Verteiler) Laufen(ctx context.Context) {
	takt := time.NewTicker(time.Second)
	defer takt.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-takt.C:
			v.messen()
			if n++; n%3 == 0 {
				v.Aktualisieren()
			}
		}
	}
}

// Aktualisieren übernimmt neue Adapter und entfernt verschwundene.
func (v *Verteiler) Aktualisieren() {
	gefunden, err := v.finden()
	if err != nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	da := map[string]bool{}
	for _, w := range gefunden {
		da[w.Schluessel] = true
		if p, ok := v.pfade[w.Schluessel]; ok {
			p.Weg = w
			continue
		}
		v.pfade[w.Schluessel] = &Pfad{Weg: w}
	}
	for k, p := range v.pfade {
		if !da[k] && p.offen.Load() == 0 {
			delete(v.pfade, k)
		}
	}
}

func (v *Verteiler) messen() {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, p := range v.pfade {
		r, s := p.rein.Load(), p.raus.Load()
		// leicht geglättet, damit die Anzeige nicht springt
		p.rateRein = 0.5*p.rateRein + 0.5*float64(r-p.letzteRein)
		p.rateRaus = 0.5*p.rateRaus + 0.5*float64(s-p.letzteRaus)
		p.letzteRein, p.letzteRaus = r, s
		// Spitze klingt langsam ab, damit ein Weg, der schneller geworden ist, wieder gewinnt
		p.spitze *= 0.995
		if p.rateRein > p.spitze {
			p.spitze = p.rateRein
		}
	}
}

// Schalten nimmt einen Adapter in die Verteilung auf oder heraus.
func (v *Verteiler) Schalten(name string, an bool) {
	v.mu.Lock()
	if an {
		delete(v.aus, name)
	} else {
		v.aus[name] = true
	}
	liste := make([]string, 0, len(v.aus))
	for k := range v.aus {
		liste = append(liste, k)
	}
	v.mu.Unlock()
	sort.Strings(liste)
	if v.beiAenderung != nil {
		v.beiAenderung(liste)
	}
}

// Pfade liefert die benutzbaren Wege, der günstigste zuerst. Günstig heißt: wenige
// offene Verbindungen im Verhältnis zu dem, was der Weg zuletzt geschafft hat.
func (v *Verteiler) Pfade() []*Pfad {
	v.mu.Lock()
	defer v.mu.Unlock()
	jetzt := time.Now().UnixNano()
	var gut, schlecht []*Pfad
	for _, p := range v.pfade {
		if v.aus[p.Name] {
			continue
		}
		if p.gestoert.Load() > jetzt {
			schlecht = append(schlecht, p)
		} else {
			gut = append(gut, p)
		}
	}
	kosten := func(p *Pfad) float64 {
		// 1 MB/s Grundgewicht, damit neue Wege ohne Messwert auch Verbindungen bekommen
		return float64(p.offen.Load()+1) / (p.spitze + 1e6)
	}
	sort.Slice(gut, func(i, j int) bool {
		ki, kj := kosten(gut[i]), kosten(gut[j])
		if ki != kj {
			return ki < kj
		}
		// Gleichstand: der am längsten nicht benutzte zuerst, so wechseln sich die Wege ab
		return gut[i].zuletzt.Load() < gut[j].zuletzt.Load()
	})
	return append(gut, schlecht...)
}

// Waehlen verbindet über den günstigsten Weg und weicht bei Fehlern auf die anderen aus.
func (v *Verteiler) Waehlen(ctx context.Context, adresse string) (net.Conn, *Pfad, error) {
	host, port, err := net.SplitHostPort(adresse)
	if err != nil {
		return nil, nil, err
	}
	pfade := v.Pfade()
	if len(pfade) == 0 {
		return nil, nil, errors.New("kein Weg ins Internet")
	}
	var letzter error
	for _, p := range pfade {
		ips, err := p.Aufloesen(ctx, host)
		if err != nil {
			letzter = err
			continue
		}
		for _, ip := range ips {
			c, err := p.Weg.Waehlen(ctx, net.JoinHostPort(ip.String(), port))
			if err == nil {
				p.gestoert.Store(0)
				return v.Zaehlen(c, p), p, nil
			}
			letzter = err
			if ctx.Err() != nil {
				return nil, nil, err
			}
		}
		// 20 s meiden, danach wieder probieren
		p.gestoert.Store(time.Now().Add(20 * time.Second).UnixNano())
		v.mu.Lock()
		p.fehler = fmt.Sprint(letzter)
		v.mu.Unlock()
	}
	return nil, nil, letzter
}

// WaehlenUeber verbindet über genau einen Weg (für den Downloader).
func (v *Verteiler) WaehlenUeber(ctx context.Context, p *Pfad, adresse string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(adresse)
	if err != nil {
		return nil, err
	}
	ips, err := p.Aufloesen(ctx, host)
	if err != nil {
		return nil, err
	}
	var letzter error
	for _, ip := range ips {
		c, err := p.Weg.Waehlen(ctx, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return v.Zaehlen(c, p), nil
		}
		letzter = err
	}
	return nil, letzter
}

// Zaehlen hängt die Byte-Zähler des Wegs an eine Verbindung.
func (v *Verteiler) Zaehlen(c net.Conn, p *Pfad) net.Conn {
	p.offen.Add(1)
	p.zuletzt.Store(time.Now().UnixNano())
	return &gezaehlt{Conn: c, p: p}
}

// Stand liefert den Zustand aller Wege für die Oberfläche.
func (v *Verteiler) Stand() []Stand {
	v.mu.Lock()
	defer v.mu.Unlock()
	jetzt := time.Now().UnixNano()
	var s []Stand
	for _, p := range v.pfade {
		g := p.gestoert.Load() > jetzt
		st := Stand{
			Weg:        p.Weg,
			Aktiv:      !v.aus[p.Name],
			Gestoert:   g,
			Offen:      p.offen.Load(),
			MbitRein:   p.rateRein * 8 / 1e6,
			MbitRaus:   p.rateRaus * 8 / 1e6,
			MbitSpitze: p.spitze * 8 / 1e6,
			GBRein:     float64(p.rein.Load()) / 1e9,
			GBRaus:     float64(p.raus.Load()) / 1e9,
		}
		if g {
			st.Fehler = p.fehler
		}
		s = append(s, st)
	}
	sort.Slice(s, func(i, j int) bool { return s[i].Schluessel < s[j].Schluessel })
	return s
}

type gezaehlt struct {
	net.Conn
	p  *Pfad
	zu sync.Once
}

func (g *gezaehlt) Read(b []byte) (int, error) {
	n, err := g.Conn.Read(b)
	g.p.rein.Add(int64(n))
	return n, err
}

func (g *gezaehlt) Write(b []byte) (int, error) {
	n, err := g.Conn.Write(b)
	g.p.raus.Add(int64(n))
	return n, err
}

func (g *gezaehlt) Close() error {
	g.zu.Do(func() { g.p.offen.Add(-1) })
	return g.Conn.Close()
}

// CloseWrite reicht ein halbes Schließen an TCP durch, falls möglich.
func (g *gezaehlt) CloseWrite() error {
	if cw, ok := g.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

var _ io.ReadWriteCloser = (*gezaehlt)(nil)
