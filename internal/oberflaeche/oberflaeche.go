// Paket oberflaeche liefert die lokale Weboberfläche und ihre kleine API.
package oberflaeche

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/Dschonas04/bondr/internal/einstellungen"
	"github.com/Dschonas04/bondr/internal/laden"
	"github.com/Dschonas04/bondr/internal/system"
	"github.com/Dschonas04/bondr/internal/verteiler"
)

//go:embed web
var dateien embed.FS

// Oberflaeche bündelt, was die Seite braucht.
type Oberflaeche struct {
	V            *verteiler.Verteiler
	L            *laden.Lader
	E            *einstellungen.Speicher
	ProxyAdresse string
	Version      string
	Beenden      func()

	schluessel string
}

// Schluessel ist ein zufälliges Kennwort in der Adresse. Ohne ihn nimmt die API nichts
// an, damit fremde Webseiten im Browser nicht über localhost umschalten können.
func (o *Oberflaeche) Schluessel() string {
	if o.schluessel == "" {
		b := make([]byte, 16)
		rand.Read(b)
		o.schluessel = hex.EncodeToString(b)
	}
	return o.schluessel
}

// Handler liefert alle Routen.
func (o *Oberflaeche) Handler() http.Handler {
	o.Schluessel()
	web, _ := fs.Sub(dateien, "web")
	m := http.NewServeMux()
	m.Handle("GET /", http.FileServerFS(web))
	m.HandleFunc("GET /api/stand", o.pruefen(o.stand))
	m.HandleFunc("POST /api/weg", o.pruefen(o.weg))
	m.HandleFunc("POST /api/systemproxy", o.pruefen(o.systemproxy))
	m.HandleFunc("POST /api/laden", o.pruefen(o.laden))
	m.HandleFunc("POST /api/abbrechen", o.pruefen(o.abbrechen))
	m.HandleFunc("POST /api/zeigen", o.pruefen(o.zeigen))
	m.HandleFunc("POST /api/ordner", o.pruefen(o.ordner))
	m.HandleFunc("POST /api/beenden", o.pruefen(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		go o.Beenden()
	}))
	return sicher(m)
}

func sicher(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Nur Aufrufe über 127.0.0.1/localhost, kein DNS-Rebinding
		host := r.Host
		if i := strings.LastIndex(host, ":"); i >= 0 {
			host = host[:i]
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "nur lokal", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func (o *Oberflaeche) pruefen(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Bondr") != o.schluessel {
			http.Error(w, "Schlüssel fehlt", http.StatusForbidden)
			return
		}
		f(w, r)
	}
}

func json200(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (o *Oberflaeche) stand(w http.ResponseWriter, r *http.Request) {
	e := o.E.Werte()
	var summeRein, summeRaus float64
	wege := o.V.Stand()
	for _, s := range wege {
		summeRein += s.MbitRein
		summeRaus += s.MbitRaus
	}
	json200(w, map[string]any{
		"version":     o.Version,
		"wege":        wege,
		"downloads":   o.L.Liste(),
		"proxy":       o.ProxyAdresse,
		"systemproxy": e.Systemproxy && system.ProxyAktiv(o.ProxyAdresse),
		"gewuenscht":  e.Systemproxy,
		"ordner":      e.Ordner,
		"mbit_rein":   summeRein,
		"mbit_raus":   summeRaus,
	})
}

func (o *Oberflaeche) weg(w http.ResponseWriter, r *http.Request) {
	var a struct {
		Name string `json:"name"`
		An   bool   `json:"an"`
	}
	if json.NewDecoder(r.Body).Decode(&a) != nil || a.Name == "" {
		http.Error(w, "ungültig", http.StatusBadRequest)
		return
	}
	o.V.Schalten(a.Name, a.An)
	w.WriteHeader(http.StatusNoContent)
}

func (o *Oberflaeche) systemproxy(w http.ResponseWriter, r *http.Request) {
	var a struct {
		An bool `json:"an"`
	}
	if json.NewDecoder(r.Body).Decode(&a) != nil {
		http.Error(w, "ungültig", http.StatusBadRequest)
		return
	}
	if err := SystemproxySchalten(o.E, o.ProxyAdresse, a.An); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SystemproxySchalten setzt Windows-Proxy, Autostart und Einstellung zusammen.
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
	if err := system.Autostart(an); err != nil {
		return err
	}
	return e.Aendern(func(w *einstellungen.Werte) { w.Systemproxy = an })
}

func (o *Oberflaeche) laden(w http.ResponseWriter, r *http.Request) {
	var a struct {
		URL string `json:"url"`
	}
	if json.NewDecoder(r.Body).Decode(&a) != nil {
		http.Error(w, "ungültig", http.StatusBadRequest)
		return
	}
	if _, err := o.L.Starten(a.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (o *Oberflaeche) abbrechen(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	o.L.Abbrechen(id)
	w.WriteHeader(http.StatusNoContent)
}

func (o *Oberflaeche) zeigen(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	for _, a := range o.L.Liste() {
		if a.ID == id && a.Status == "fertig" {
			system.OrdnerZeigen(a.Datei)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (o *Oberflaeche) ordner(w http.ResponseWriter, r *http.Request) {
	var a struct {
		Ordner string `json:"ordner"`
	}
	if json.NewDecoder(r.Body).Decode(&a) != nil || strings.TrimSpace(a.Ordner) == "" {
		http.Error(w, "ungültig", http.StatusBadRequest)
		return
	}
	o.E.Aendern(func(w *einstellungen.Werte) { w.Ordner = strings.TrimSpace(a.Ordner) })
	w.WriteHeader(http.StatusNoContent)
}
