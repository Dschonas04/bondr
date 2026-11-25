// Paket api ist die lokale Steuerung für die Apps (Windows, Mac, Linux). Sie lauscht nur auf
// 127.0.0.1 und verlangt einen Schlüssel, der in einer nur für den Nutzer lesbaren Datei
// liegt; fremde Programme oder Webseiten können Bondr so nicht umschalten.
package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Dschonas04/bondr/internal/einstellungen"
	"github.com/Dschonas04/bondr/internal/laden"
	"github.com/Dschonas04/bondr/internal/system"
	"github.com/Dschonas04/bondr/internal/umleitung"
	"github.com/Dschonas04/bondr/internal/verteiler"
)

// API bündelt, was die Steuerung braucht.
type API struct {
	V            *verteiler.Verteiler
	L            *laden.Lader
	E            *einstellungen.Speicher
	ProxyAdresse string
	ProxyBereit  bool // Proxy-Port gehört bondrd
	Version      string
	Beenden      func()
	schluessel   string
}

// SchluesselDatei ist der Ort, an dem die Apps den Schlüssel finden.
func SchluesselDatei() string { return filepath.Join(einstellungen.Ordner(), "api-schluessel") }

// SchluesselSchreiben erzeugt einen neuen Schlüssel und legt ihn ab.
func (a *API) SchluesselSchreiben() error {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	a.schluessel = hex.EncodeToString(b)
	os.MkdirAll(einstellungen.Ordner(), 0o700)
	return os.WriteFile(SchluesselDatei(), []byte(a.schluessel), 0o600)
}

// Handler liefert alle Routen.
func (a *API) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /stand", a.stand)
	m.HandleFunc("POST /regel", a.regel)
	m.HandleFunc("POST /strategie", a.strategie)
	m.HandleFunc("POST /systemproxy", a.systemproxy)
	m.HandleFunc("POST /laden", a.laden)
	m.HandleFunc("POST /abbrechen", a.abbrechen)
	m.HandleFunc("POST /ordner", a.ordner)
	m.HandleFunc("POST /umleitung", a.umleitung)
	m.HandleFunc("POST /beenden", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		go a.Beenden()
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if i := strings.LastIndex(host, ":"); i >= 0 {
			host = host[:i]
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "nur lokal", http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Bondr")), []byte(a.schluessel)) != 1 {
			http.Error(w, "Schlüssel fehlt", http.StatusForbidden)
			return
		}
		m.ServeHTTP(w, r)
	})
}

func antworten(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func lesen(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil {
		http.Error(w, "ungültige Anfrage", http.StatusBadRequest)
		return false
	}
	return true
}

func (a *API) speichern(w http.ResponseWriter, f func(*einstellungen.Werte)) {
	if err := a.E.Aendern(f); err != nil {
		http.Error(w, "Nicht gespeichert: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) stand(w http.ResponseWriter, r *http.Request) {
	e := a.E.Werte()
	wege := a.V.Stand()
	var rein, raus float64
	for _, s := range wege {
		rein += s.MbitRein
		raus += s.MbitRaus
	}
	dl := a.L.Liste()
	if dl == nil {
		dl = []laden.Eintrag{}
	}
	if wege == nil {
		wege = []verteiler.Stand{}
	}
	antworten(w, map[string]any{
		"version":      a.Version,
		"strategie":    e.Strategie,
		"systemproxy":  e.Systemproxy && system.ProxyAktiv(a.ProxyAdresse),
		"gewuenscht":   e.Systemproxy,
		"proxy":        a.ProxyAdresse,
		"proxy_bereit": a.ProxyBereit,
		"ordner":       e.Ordner,
		"mbit_rein":    rein,
		"mbit_raus":    raus,
		"wege":         wege,
		"downloads":    dl,
		"umleitung":    umleitung.Stand(e),
	})
}

func (a *API) regel(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Name    string `json:"name"`
		Gewicht int    `json:"gewicht"`
		Rolle   string `json:"rolle"`
	}
	if !lesen(w, r, &q) {
		return
	}
	switch q.Rolle {
	case einstellungen.RolleHaupt, einstellungen.RolleReserve, einstellungen.RolleAus:
	default:
		http.Error(w, "Rolle muss haupt, reserve oder aus sein", http.StatusBadRequest)
		return
	}
	if q.Name == "" || q.Gewicht < 1 || q.Gewicht > 10 {
		http.Error(w, "Name fehlt oder Gewicht nicht zwischen 1 und 10", http.StatusBadRequest)
		return
	}
	a.speichern(w, func(v *einstellungen.Werte) {
		v.Regeln[q.Name] = einstellungen.Regel{Gewicht: q.Gewicht, Rolle: q.Rolle}
	})
}

func (a *API) strategie(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Strategie string `json:"strategie"`
	}
	if !lesen(w, r, &q) {
		return
	}
	switch q.Strategie {
	case einstellungen.StrategieWenigste, einstellungen.StrategieReihum, einstellungen.StrategieAusfall:
	default:
		http.Error(w, "unbekannte Strategie", http.StatusBadRequest)
		return
	}
	a.speichern(w, func(v *einstellungen.Werte) { v.Strategie = q.Strategie })
}

// SystemproxySchalten setzt den Systemproxy und merkt sich die Einstellung.
func SystemproxySchalten(e *einstellungen.Speicher, adresse string, an bool) error {
	var err error
	if an {
		err = system.ProxySetzen(adresse)
	} else {
		err = system.ProxyEntfernen(adresse)
	}
	if err != nil {
		return err
	}
	return e.Aendern(func(w *einstellungen.Werte) { w.Systemproxy = an })
}

func (a *API) systemproxy(w http.ResponseWriter, r *http.Request) {
	var q struct {
		An bool `json:"an"`
	}
	if !lesen(w, r, &q) {
		return
	}
	if q.An && !a.ProxyBereit {
		http.Error(w, "Der Proxy-Port "+a.ProxyAdresse+" ist von einem anderen Programm belegt; der Systemproxy bleibt aus.", http.StatusConflict)
		return
	}
	if err := SystemproxySchalten(a.E, a.ProxyAdresse, q.An); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) laden(w http.ResponseWriter, r *http.Request) {
	var q struct {
		URL string `json:"url"`
	}
	if !lesen(w, r, &q) {
		return
	}
	if _, err := a.L.Starten(q.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) abbrechen(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID int `json:"id"`
	}
	if !lesen(w, r, &q) {
		return
	}
	a.L.Abbrechen(q.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ordner(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Ordner string `json:"ordner"`
	}
	if !lesen(w, r, &q) {
		return
	}
	ordner := strings.TrimSpace(q.Ordner)
	if ordner == "" {
		http.Error(w, "Ordner fehlt", http.StatusBadRequest)
		return
	}
	a.speichern(w, func(v *einstellungen.Werte) { v.Ordner = ordner })
}

func (a *API) umleitung(w http.ResponseWriter, r *http.Request) {
	var q struct {
		An        bool    `json:"an"`
		Programme *string `json:"programme"`
	}
	if !lesen(w, r, &q) {
		return
	}
	var liste []string
	if q.Programme != nil {
		liste = umleitung.Programme(*q.Programme)
		if len(liste) == 0 {
			http.Error(w, "Mindestens ein Programm angeben, z. B. steam.exe", http.StatusBadRequest)
			return
		}
	}
	a.speichern(w, func(v *einstellungen.Werte) {
		v.Umleitung = q.An
		if liste != nil {
			v.Programme = liste
		}
	})
}
