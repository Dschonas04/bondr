// Paket laden holt eine Datei in Stücken gleichzeitig über alle Wege. Das ist der
// Fall, in dem sich die Bandbreite wirklich addiert: ein einzelner Download.
package laden

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Dschonas04/bondr/internal/verteiler"
)

const (
	stueck       = 4 << 20          // kleinere Dateien in einem Stück
	minTeil      = 1 << 20          // kleiner wird kein Abschnitt geteilt
	jeWeg        = 4                // höchstens so viele Verbindungen je Weg (Gewicht 10)
	fehlerProWeg = 5                // danach hört ein Weg für diesen Download auf
	stillstand   = 20 * time.Second // ohne neue Daten so lange, dann neu versuchen
)

// Auftrag ist ein laufender oder fertiger Download.
type Auftrag struct {
	ID      int
	URL     string
	Datei   string
	Groesse int64
	Status  string // läuft, fertig, Fehler, abgebrochen
	Fehler  string
	Start   time.Time
	Ende    time.Time

	geladen atomic.Int64
	anteile sync.Map // Weg-Name → *atomic.Int64
	abbruch context.CancelFunc
}

// Eintrag ist eine Momentaufnahme eines Auftrags für die Oberfläche.
type Eintrag struct {
	ID      int         `json:"id"`
	URL     string      `json:"url"`
	Datei   string      `json:"datei"`
	Groesse int64       `json:"groesse"`
	Geladen int64       `json:"geladen"`
	Status  string      `json:"status"`
	Fehler  string      `json:"fehler,omitempty"`
	Wege    []WegAnteil `json:"wege"`
	Mbit    float64     `json:"mbit"`
	Start   time.Time   `json:"start"`
	Ende    time.Time   `json:"ende,omitempty"`
}

// WegAnteil sagt, wie viel ein Weg zu diesem Download beigetragen hat.
type WegAnteil struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// Lader verwaltet alle Downloads.
type Lader struct {
	V      *verteiler.Verteiler
	Ordner func() string

	mu        sync.Mutex
	auftraege []*Auftrag
	naechste  int
}

// Starten beginnt einen Download im Hintergrund.
func (l *Lader) Starten(adresse string) (*Auftrag, error) {
	u, err := url.Parse(strings.TrimSpace(adresse))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("Adresse muss mit http:// oder https:// beginnen")
	}
	ctx, abbruch := context.WithCancel(context.Background())
	l.mu.Lock()
	l.naechste++
	a := &Auftrag{ID: l.naechste, URL: u.String(), Status: "läuft", Start: time.Now(), abbruch: abbruch}
	l.auftraege = append([]*Auftrag{a}, l.auftraege...)
	if len(l.auftraege) > 30 {
		l.auftraege = l.auftraege[:30]
	}
	l.mu.Unlock()
	go func() {
		err := l.laden(ctx, a, u)
		l.mu.Lock()
		defer l.mu.Unlock()
		a.Ende = time.Now()
		switch {
		case ctx.Err() != nil:
			a.Status = "abgebrochen"
		case err != nil:
			a.Status, a.Fehler = "Fehler", err.Error()
		default:
			a.Status = "fertig"
		}
	}()
	return a, nil
}

// Abbrechen stoppt einen Download.
func (l *Lader) Abbrechen(id int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, a := range l.auftraege {
		if a.ID == id && a.abbruch != nil {
			a.abbruch()
		}
	}
}

// Liste liefert eine Momentaufnahme aller Downloads.
func (l *Lader) Liste() []Eintrag {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Eintrag, 0, len(l.auftraege))
	for _, a := range l.auftraege {
		k := Eintrag{ID: a.ID, URL: a.URL, Datei: a.Datei, Groesse: a.Groesse, Status: a.Status, Fehler: a.Fehler, Start: a.Start, Ende: a.Ende}
		k.Geladen = a.geladen.Load()
		a.anteile.Range(func(n, b any) bool {
			k.Wege = append(k.Wege, WegAnteil{Name: n.(string), Bytes: b.(*atomic.Int64).Load()})
			return true
		})
		ende := a.Ende
		if ende.IsZero() {
			ende = time.Now()
		}
		if d := ende.Sub(a.Start).Seconds(); d > 0 {
			k.Mbit = float64(k.Geladen) * 8 / 1e6 / d
		}
		out = append(out, k)
	}
	return out
}

func (a *Auftrag) anteil(name string) *atomic.Int64 {
	z, _ := a.anteile.LoadOrStore(name, new(atomic.Int64))
	return z.(*atomic.Int64)
}

// client baut einen HTTP-Client, der nur über diesen Weg geht.
func (l *Lader) client(p *verteiler.Pfad) *http.Client {
	t := &http.Transport{
		DialContext: func(ctx context.Context, _, adr string) (net.Conn, error) {
			return l.V.WaehlenUeber(ctx, p, adr)
		},
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:     false, // je Anfrage eine eigene Verbindung, sonst bündelt HTTP/2 alles auf eine
		MaxIdleConnsPerHost:   jeWeg,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	return &http.Client{Transport: t}
}

func (l *Lader) laden(ctx context.Context, a *Auftrag, u *url.URL) error {
	pfade, anzahl := l.V.LadePfade(jeWeg)
	if len(pfade) == 0 {
		return errors.New("kein Weg ins Internet")
	}
	// Erst fragen, wie groß die Datei ist und ob Teilabrufe gehen.
	erster := l.client(pfade[0])
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("User-Agent", "Bondr/1")
	antw, err := erster.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, io.LimitReader(antw.Body, 1<<16))
	antw.Body.Close()
	if antw.StatusCode >= 400 {
		return fmt.Errorf("Server antwortet %s", antw.Status)
	}
	endURL := antw.Request.URL // nach Weiterleitungen
	name := dateiname(antw, endURL)
	ziel, err := freierName(l.Ordner(), name)
	if err != nil {
		return err
	}
	l.mu.Lock()
	a.Datei = ziel
	l.mu.Unlock()

	groesse := int64(-1)
	if antw.StatusCode == http.StatusPartialContent {
		if cr := antw.Header.Get("Content-Range"); cr != "" {
			if i := strings.LastIndex(cr, "/"); i >= 0 {
				groesse, _ = strconv.ParseInt(cr[i+1:], 10, 64)
			}
		}
	}
	teil := ziel + ".teil"
	f, err := os.Create(teil)
	if err != nil {
		return err
	}
	if groesse > stueck {
		l.mu.Lock()
		a.Groesse = groesse
		l.mu.Unlock()
		if err = f.Truncate(groesse); err == nil {
			err = l.inStuecken(ctx, a, f, pfade, anzahl, endURL, groesse)
		}
	} else {
		err = l.amStueck(ctx, a, f, pfade[0], endURL)
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		os.Remove(teil)
		return err
	}
	return os.Rename(teil, ziel)
}

// amStueck lädt ohne Teilabrufe über einen Weg (Server kann keine Ranges).
func (l *Lader) amStueck(ctx context.Context, a *Auftrag, f *os.File, p *verteiler.Pfad, u *url.URL) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("User-Agent", "Bondr/1")
	antw, err := l.client(p).Do(req)
	if err != nil {
		return err
	}
	defer antw.Body.Close()
	if antw.StatusCode != http.StatusOK {
		return fmt.Errorf("Server antwortet %s", antw.Status)
	}
	if antw.ContentLength > 0 {
		l.mu.Lock()
		a.Groesse = antw.ContentLength
		l.mu.Unlock()
	}
	_, err = io.Copy(f, &zaehler{r: antw.Body, a: a, w: a.anteil(p.Name)})
	return err
}

// abschnitt ist ein Bereich [pos, ende) der Datei. Eine Verbindung liest ihn von vorn;
// wird ein anderer Weg frei, nimmt er sich die hintere Hälfte (ende wird verkürzt).
type abschnitt struct {
	pos, ende int64
	belegt    bool
}

type plan struct {
	mu        sync.Mutex
	teile     []*abschnitt
	geaendert chan struct{}
}

// nehmen liefert einen freien Abschnitt oder teilt den größten laufenden.
func (pl *plan) nehmen() *abschnitt {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	var groesster *abschnitt
	for _, t := range pl.teile {
		if t.pos >= t.ende {
			continue
		}
		if !t.belegt {
			t.belegt = true
			return t
		}
		if groesster == nil || t.ende-t.pos > groesster.ende-groesster.pos {
			groesster = t
		}
	}
	if groesster == nil || groesster.ende-groesster.pos < 2*minTeil {
		return nil
	}
	mitte := groesster.pos + (groesster.ende-groesster.pos)/2
	n := &abschnitt{pos: mitte, ende: groesster.ende, belegt: true}
	groesster.ende = mitte
	pl.teile = append(pl.teile, n)
	return n
}

func (pl *plan) freigeben(t *abschnitt) {
	pl.mu.Lock()
	t.belegt = false
	pl.mu.Unlock()
}

func (pl *plan) fertig() bool {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	for _, t := range pl.teile {
		if t.pos < t.ende {
			return false
		}
	}
	return true
}

func (l *Lader) inStuecken(ctx context.Context, a *Auftrag, f *os.File, pfade []*verteiler.Pfad, anzahl []int, u *url.URL, groesse int64) error {
	// Anfangs ein Abschnitt je Verbindung; danach teilen sich freie Verbindungen
	// die Arbeit der langsamen. So bleibt die Zahl der Anfragen klein, was Server
	// mit Anfragebegrenzung (429) schont.
	verbindungen := 0
	for _, n := range anzahl {
		verbindungen += n
	}
	anfang := int(min(int64(verbindungen), max(1, groesse/minTeil)))
	pl := &plan{}
	for i := 0; i < anfang; i++ {
		von := groesse * int64(i) / int64(anfang)
		bis := groesse * int64(i+1) / int64(anfang)
		pl.teile = append(pl.teile, &abschnitt{pos: von, ende: bis})
	}
	var wg sync.WaitGroup
	var fehlerMu sync.Mutex
	var letzter error
	ctx, stopp := context.WithCancel(ctx)
	defer stopp()

	for pi, p := range pfade {
		c := l.client(p)
		var fehlschlaege, aktiv atomic.Int64
		for n := 0; n < anzahl[pi]; n++ {
			wg.Add(1)
			aktiv.Add(1)
			go func(p *verteiler.Pfad) {
				defer wg.Done()
				defer aktiv.Add(-1)
				for ctx.Err() == nil && fehlschlaege.Load() < fehlerProWeg {
					t := pl.nehmen()
					if t == nil {
						if pl.fertig() {
							stopp()
							return
						}
						select {
						case <-ctx.Done():
							return
						case <-time.After(200 * time.Millisecond):
						}
						continue
					}
					err := l.holen(ctx, a, c, p, u, f, pl, t)
					pl.freigeben(t)
					if err == nil {
						fehlschlaege.Store(0)
						continue
					}
					if ctx.Err() != nil {
						return
					}
					log.Printf("Abschnitt über %s: %v", p.Name, ohneURL(err))
					warten := time.Second
					var b *bremse
					if errors.As(err, &b) {
						warten = b.warten
						if aktiv.Load() > 1 {
							return // Server will weniger gleichzeitige Anfragen
						}
					} else {
						fehlschlaege.Add(1)
						fehlerMu.Lock()
						letzter = fmt.Errorf("%s: %w", p.Name, err)
						fehlerMu.Unlock()
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(warten):
					}
				}
			}(p)
		}
	}
	wg.Wait()
	if pl.fertig() {
		return nil
	}
	if letzter == nil {
		letzter = context.Canceled
	}
	return letzter
}

// holen liest einen Abschnitt ab t.pos, bis t.ende erreicht ist (das sich beim
// Teilen verkürzen kann). Bei Fehlern bleibt t.pos stehen, der Rest wird neu geholt.
func (l *Lader) holen(ctx context.Context, a *Auftrag, c *http.Client, p *verteiler.Pfad, u *url.URL, f *os.File, pl *plan, t *abschnitt) error {
	ctx, abbruch := context.WithCancel(ctx)
	defer abbruch()
	wache := time.AfterFunc(stillstand, abbruch)
	defer wache.Stop()
	pl.mu.Lock()
	von, bis := t.pos, t.ende-1
	pl.mu.Unlock()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", von, bis))
	req.Header.Set("User-Agent", "Bondr/1")
	antw, err := c.Do(req)
	if err != nil {
		return err
	}
	defer antw.Body.Close()
	if antw.StatusCode == http.StatusTooManyRequests || antw.StatusCode == http.StatusServiceUnavailable {
		b := &bremse{warten: 3 * time.Second}
		if s, err := strconv.Atoi(antw.Header.Get("Retry-After")); err == nil && s > 0 && s < 120 {
			b.warten = time.Duration(s) * time.Second
		}
		return b
	}
	if antw.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("Server antwortet %s statt Teilinhalt", antw.Status)
	}
	w := a.anteil(p.Name)
	puffer := make([]byte, 256<<10)
	for {
		n, err := antw.Body.Read(puffer)
		if n > 0 {
			wache.Reset(stillstand)
			pl.mu.Lock()
			pos, ende := t.pos, t.ende
			pl.mu.Unlock()
			if int64(n) > ende-pos {
				n = int(max(0, ende-pos))
			}
			if n > 0 {
				if _, e := f.WriteAt(puffer[:n], pos); e != nil {
					return e
				}
				pl.mu.Lock()
				t.pos += int64(n)
				pos, ende = t.pos, t.ende
				pl.mu.Unlock()
				a.geladen.Add(int64(n))
				w.Add(int64(n))
			}
			if pos >= ende {
				return nil // fertig, oder ein anderer hat den Rest übernommen
			}
		}
		if err != nil {
			if err == io.EOF {
				return io.ErrUnexpectedEOF
			}
			return err
		}
	}
}

// ohneURL entfernt die Adresse aus Fehlern, damit Schlüssel in der Abfrage
// (signierte Links) nicht im Protokoll landen.
func ohneURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}

// bremse heißt: Server meldet 429 oder 503, später noch einmal versuchen.
type bremse struct{ warten time.Duration }

func (b *bremse) Error() string { return "Server bremst" }

type zaehler struct {
	r io.Reader
	a *Auftrag
	w *atomic.Int64
}

func (z *zaehler) Read(b []byte) (int, error) {
	n, err := z.r.Read(b)
	z.a.geladen.Add(int64(n))
	z.w.Add(int64(n))
	return n, err
}

func dateiname(antw *http.Response, u *url.URL) string {
	if cd := antw.Header.Get("Content-Disposition"); cd != "" {
		if _, p, err := mime.ParseMediaType(cd); err == nil && p["filename"] != "" {
			return sauber(p["filename"])
		}
	}
	n := path.Base(u.Path)
	if n == "/" || n == "." || n == "" {
		n = u.Hostname() + ".bin"
	}
	if d, err := url.PathUnescape(n); err == nil {
		n = d
	}
	return sauber(n)
}

func sauber(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, n)
	n = strings.Trim(n, ". ")
	if n == "" {
		n = "download.bin"
	}
	return n
}

func freierName(ordner, name string) (string, error) {
	if err := os.MkdirAll(ordner, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(name)
	stamm := strings.TrimSuffix(name, ext)
	for i := 0; i < 1000; i++ {
		k := name
		if i > 0 {
			k = fmt.Sprintf("%s (%d)%s", stamm, i, ext)
		}
		p := filepath.Join(ordner, k)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if _, err := os.Stat(p + ".teil"); os.IsNotExist(err) {
				return p, nil
			}
		}
	}
	return "", errors.New("kein freier Dateiname")
}
