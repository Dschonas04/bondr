// Paket verteiler ist der Lastverteiler: Er entscheidet für jede neue Verbindung, über
// welchen Weg sie läuft, ähnlich einem nginx-Upstream mit weight, backup und
// Gesundheitsprüfung, und zählt, wie viel über jeden Weg geht.
package verteiler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Dschonas04/bondr/internal/einstellungen"
	"github.com/Dschonas04/bondr/internal/wege"
)

// Regeln liefert Gewicht, Rolle und Strategie.
type Regeln interface {
	Regel(name, art string) einstellungen.Regel
	Strategie() string
}

type vorgaben struct{}

func (vorgaben) Regel(_, art string) einstellungen.Regel { return einstellungen.Vorgabe(art) }
func (vorgaben) Strategie() string                       { return einstellungen.StrategieWenigste }

// Pfad ist ein Weg mit seinem Laufzeitzustand.
type Pfad struct {
	wege.Weg
	offen    atomic.Int64
	zuletzt  atomic.Int64
	rein     atomic.Int64
	raus     atomic.Int64
	gestoert atomic.Int64 // Unix-Nanosekunden, bis wann der Weg gemieden wird
	fehler   atomic.Value // string

	// nur unter Verteiler.mu
	letzteRein, letzteRaus int64
	rateRein, rateRaus     float64
	spitze                 float64
	aktuell                int // für gewichtetes Reihum (nginx smooth weighted round robin)
	geprueft               time.Time
	pruefOK                bool
}

// Gestoert sagt, ob der Weg gerade gemieden wird.
func (p *Pfad) Gestoert() bool { return p.gestoert.Load() > time.Now().UnixNano() }

func (p *Pfad) stoeren(d time.Duration, err error) {
	p.gestoert.Store(time.Now().Add(d).UnixNano())
	p.fehler.Store(fmt.Sprint(err))
}

// Stand ist, was die Oberfläche je Weg anzeigt.
type Stand struct {
	wege.Weg
	Gewicht    int     `json:"gewicht"`
	Rolle      string  `json:"rolle"`
	Benutzt    bool    `json:"benutzt"` // bekommt gerade neue Verbindungen
	Gestoert   bool    `json:"gestoert"`
	Fehler     string  `json:"fehler,omitempty"`
	Offen      int64   `json:"offen"`
	MbitRein   float64 `json:"mbit_rein"`
	MbitRaus   float64 `json:"mbit_raus"`
	MbitSpitze float64 `json:"mbit_spitze"`
	MBRein     float64 `json:"mb_rein"`
	MBRaus     float64 `json:"mb_raus"`
}

// Verteiler hält alle bekannten Wege.
type Verteiler struct {
	mu     sync.Mutex
	pfade  map[string]*Pfad
	finden func() ([]wege.Weg, error)
	regeln Regeln
	// Ziel der Gesundheitsprüfung; im Test austauschbar
	PruefZiele []string
}

// Neu legt einen Verteiler mit der echten Adaptersuche an.
func Neu(r Regeln) *Verteiler { return NeuMit(wege.Finden, r) }

// NeuMit nimmt eine eigene Suchfunktion, zum Testen.
func NeuMit(finden func() ([]wege.Weg, error), r Regeln) *Verteiler {
	if r == nil {
		r = vorgaben{}
	}
	v := &Verteiler{pfade: map[string]*Pfad{}, finden: finden, regeln: r,
		PruefZiele: []string{"1.1.1.1:443", "8.8.8.8:443"}}
	v.Aktualisieren()
	return v
}

// Laufen hält die Wegliste aktuell, misst die Raten und prüft die Wege.
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
			n++
			if n%3 == 0 {
				v.Aktualisieren()
			}
			if n%15 == 1 {
				go v.Pruefen(ctx)
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

// Pruefen baut über jeden Weg eine Probeverbindung auf (nginx: health_check). Ein Weg,
// der nicht durchkommt, bekommt keine neuen Verbindungen, bis die nächste Prüfung gelingt.
func (v *Verteiler) Pruefen(ctx context.Context) {
	v.mu.Lock()
	alle := make([]*Pfad, 0, len(v.pfade))
	for _, p := range v.pfade {
		alle = append(alle, p)
	}
	v.mu.Unlock()
	var wg sync.WaitGroup
	for _, p := range alle {
		wg.Add(1)
		go func(p *Pfad) {
			defer wg.Done()
			var letzter error
			ok := false
			for _, z := range v.PruefZiele {
				c, cancel := context.WithTimeout(ctx, 4*time.Second)
				conn, err := p.Weg.Waehlen(c, z)
				cancel()
				if err == nil {
					conn.Close()
					ok = true
					break
				}
				letzter = err
			}
			v.mu.Lock()
			p.geprueft, p.pruefOK = time.Now(), ok
			v.mu.Unlock()
			if ok {
				p.gestoert.Store(0)
			} else {
				p.stoeren(20*time.Second, fmt.Errorf("Prüfung: %v", letzter))
			}
		}(p)
	}
	wg.Wait()
}

func (v *Verteiler) messen() {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, p := range v.pfade {
		r, s := p.rein.Load(), p.raus.Load()
		p.rateRein = 0.5*p.rateRein + 0.5*float64(r-p.letzteRein)
		p.rateRaus = 0.5*p.rateRaus + 0.5*float64(s-p.letzteRaus)
		p.letzteRein, p.letzteRaus = r, s
		p.spitze *= 0.995
		if p.rateRein > p.spitze {
			p.spitze = p.rateRein
		}
	}
}

func (v *Verteiler) regel(p *Pfad) einstellungen.Regel {
	r := v.regeln.Regel(p.Name, p.Art)
	if r.Gewicht < 1 {
		r.Gewicht = 1
	}
	if r.Gewicht > 10 {
		r.Gewicht = 10
	}
	if r.Rolle == "" {
		r.Rolle = einstellungen.RolleHaupt
	}
	return r
}

// Pfade liefert die Wege in der Reihenfolge, in der eine neue Verbindung sie versuchen
// soll. Zuerst die gesunden Hauptwege nach Strategie; nur wenn keiner davon geht, die
// Reservewege; gestörte zuletzt, falls sich alles andere als tot erweist.
func (v *Verteiler) Pfade() []*Pfad { return v.pfadeOrdnen(false) }

// pfadeOrdnen schaltet nur bei waehlen=true das Reihum weiter, damit Anzeigen den
// Ablauf nicht verschieben.
func (v *Verteiler) pfadeOrdnen(waehlen bool) []*Pfad {
	v.mu.Lock()
	defer v.mu.Unlock()
	var haupt, reserve, schlecht []*Pfad
	for _, p := range v.pfade {
		r := v.regel(p)
		switch {
		case r.Rolle == einstellungen.RolleAus:
		case p.Gestoert():
			schlecht = append(schlecht, p)
		case r.Rolle == einstellungen.RolleReserve:
			reserve = append(reserve, p)
		default:
			haupt = append(haupt, p)
		}
	}
	nachGewicht := func(l []*Pfad) {
		sort.SliceStable(l, func(i, j int) bool {
			gi, gj := v.regel(l[i]).Gewicht, v.regel(l[j]).Gewicht
			if gi != gj {
				return gi > gj
			}
			return l[i].Schluessel < l[j].Schluessel
		})
	}
	ordnen := func(l []*Pfad) {
		if len(l) < 2 {
			return
		}
		switch v.regeln.Strategie() {
		case einstellungen.StrategieAusfall:
			nachGewicht(l)
		case einstellungen.StrategieReihum:
			if !waehlen {
				nachGewicht(l)
				return
			}
			// nginx smooth weighted round robin: jeder bekommt sein Gewicht gutgeschrieben,
			// der mit dem höchsten Stand ist dran und gibt die Summe ab.
			summe, best := 0, l[0]
			for _, p := range l {
				g := v.regel(p).Gewicht
				p.aktuell += g
				summe += g
				if p.aktuell > best.aktuell {
					best = p
				}
			}
			best.aktuell -= summe
			nachGewicht(l)
			for i, p := range l {
				if p == best {
					copy(l[1:i+1], l[:i])
					l[0] = best
					break
				}
			}
		default:
			// nginx least_conn mit Gewicht: offene Verbindungen geteilt durch Gewicht
			kosten := func(p *Pfad) float64 { return float64(p.offen.Load()+1) / float64(v.regel(p).Gewicht) }
			sort.SliceStable(l, func(i, j int) bool {
				ki, kj := kosten(l[i]), kosten(l[j])
				if ki != kj {
					return ki < kj
				}
				return l[i].zuletzt.Load() < l[j].zuletzt.Load()
			})
		}
	}
	ordnen(haupt)
	nachGewicht(reserve)
	nachGewicht(schlecht)
	out := append(haupt, reserve...)
	return append(out, schlecht...)
}

// LadePfade liefert die Wege für einen Download und wie viele Verbindungen jeder
// bekommt: Hauptwege anteilig nach Gewicht (mindestens eine), Reservewege nur, wenn
// kein Hauptweg geht. Bei „ausfall“ nur der stärkste Weg.
func (v *Verteiler) LadePfade(maxJeWeg int) ([]*Pfad, []int) {
	alle := v.Pfade()
	var nutzbar []*Pfad
	for _, p := range alle {
		if !p.Gestoert() {
			nutzbar = append(nutzbar, p)
		}
	}
	if len(nutzbar) == 0 {
		nutzbar = alle
	}
	var haupt, reserve []*Pfad
	for _, p := range nutzbar {
		if v.regel(p).Rolle == einstellungen.RolleReserve {
			reserve = append(reserve, p)
		} else {
			haupt = append(haupt, p)
		}
	}
	gewaehlt := haupt
	if len(gewaehlt) == 0 {
		gewaehlt = reserve
	}
	if len(gewaehlt) > 1 && v.regeln.Strategie() == einstellungen.StrategieAusfall {
		sort.SliceStable(gewaehlt, func(i, j int) bool { return v.regel(gewaehlt[i]).Gewicht > v.regel(gewaehlt[j]).Gewicht })
		gewaehlt = gewaehlt[:1]
	}
	maxG := 1
	for _, p := range gewaehlt {
		maxG = max(maxG, v.regel(p).Gewicht)
	}
	anzahl := make([]int, len(gewaehlt))
	for i, p := range gewaehlt {
		anzahl[i] = max(1, (maxJeWeg*v.regel(p).Gewicht+maxG/2)/maxG)
	}
	return gewaehlt, anzahl
}

// Waehlen verbindet über den ersten passenden Weg und weicht bei Fehlern aus.
func (v *Verteiler) Waehlen(ctx context.Context, adresse string) (net.Conn, *Pfad, error) {
	host, port, err := net.SplitHostPort(adresse)
	if err != nil {
		return nil, nil, err
	}
	pfade := v.pfadeOrdnen(true)
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
		p.stoeren(20*time.Second, letzter) // nginx: max_fails=1 fail_timeout=20s
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
	benutzt := map[*Pfad]bool{}
	for _, p := range v.Pfade() {
		r := v.regel(p)
		if p.Gestoert() {
			continue
		}
		if r.Rolle == einstellungen.RolleReserve && len(benutzt) > 0 {
			break
		}
		benutzt[p] = true
		if v.regeln.Strategie() == einstellungen.StrategieAusfall {
			break
		}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	var s []Stand
	for _, p := range v.pfade {
		r := v.regel(p)
		st := Stand{
			Weg:        p.Weg,
			Gewicht:    r.Gewicht,
			Rolle:      r.Rolle,
			Benutzt:    benutzt[p],
			Gestoert:   p.Gestoert(),
			Offen:      p.offen.Load(),
			MbitRein:   p.rateRein * 8 / 1e6,
			MbitRaus:   p.rateRaus * 8 / 1e6,
			MbitSpitze: p.spitze * 8 / 1e6,
			MBRein:     float64(p.rein.Load()) / 1e6,
			MBRaus:     float64(p.raus.Load()) / 1e6,
		}
		if st.Gestoert {
			st.Fehler, _ = p.fehler.Load().(string)
		}
		s = append(s, st)
	}
	sort.Slice(s, func(i, j int) bool {
		if wege.Rang(s[i].Art) != wege.Rang(s[j].Art) {
			return wege.Rang(s[i].Art) < wege.Rang(s[j].Art)
		}
		return s[i].Name < s[j].Name
	})
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
