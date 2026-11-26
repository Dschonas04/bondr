//go:build linux && !android

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Linux kennt keinen einheitlichen Systemproxy. Bondr setzt ihn dort, wo Browser und
// Programme ihn lesen: in GNOME (gsettings, auch Cinnamon, Budgie, Pantheon) und in KDE
// (kioslaverc). Firefox und Chrome folgen diesen Einstellungen.

var ausnahmen = []string{"localhost", "127.0.0.0/8", "::1", "*.local", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "*.fritz.box"}

const gs = "org.gnome.system.proxy"

func gnome() bool {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return false
	}
	return exec.Command("gsettings", "get", gs, "mode").Run() == nil
}

func gset(schema, key, wert string) error {
	if b, err := exec.Command("gsettings", "set", schema, key, wert).CombinedOutput(); err != nil {
		return fmt.Errorf("gsettings %s %s: %v %s", schema, key, err, strings.TrimSpace(string(b)))
	}
	return nil
}

func gget(schema, key string) string {
	b, _ := exec.Command("gsettings", "get", schema, key).Output()
	return strings.Trim(strings.TrimSpace(string(b)), "'")
}

func kdeDatei() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "kioslaverc")
}

// kdeWerkzeug liefert kwriteconfig6 oder kwriteconfig5, falls KDE installiert ist.
func kdeWerkzeug(art string) string {
	for _, n := range []string{"k" + art + "config6", "k" + art + "config5"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func kdeSetzen(werte map[string]string) error {
	w := kdeWerkzeug("write")
	if w == "" {
		return nil
	}
	for k, v := range werte {
		if b, err := exec.Command(w, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", k, v).CombinedOutput(); err != nil {
			return fmt.Errorf("%s %s: %v %s", filepath.Base(w), k, err, strings.TrimSpace(string(b)))
		}
	}
	// laufende KDE-Programme lesen die Einstellung neu
	exec.Command("dbus-send", "--type=signal", "/KIO/Scheduler", "org.kde.KIO.Scheduler.reparseSlaveConfiguration", "string:").Run()
	return nil
}

func kdeLesen(k string) string {
	r := kdeWerkzeug("read")
	if r == "" {
		return ""
	}
	b, _ := exec.Command(r, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", k).Output()
	return strings.TrimSpace(string(b))
}

func keinDesktop() error {
	return fmt.Errorf("weder GNOME- noch KDE-Proxyeinstellungen gefunden; Programme können Bondr direkt nutzen: SOCKS5 oder HTTP auf 127.0.0.1")
}

// ProxySetzen trägt Bondr als HTTP-, HTTPS- und SOCKS-Proxy ein.
func ProxySetzen(adresse string) error {
	host, port, _ := strings.Cut(adresse, ":")
	gesetzt := false
	if gnome() {
		liste := "['" + strings.Join(ausnahmen, "', '") + "']"
		for _, a := range [][3]string{
			{gs + ".http", "host", "'" + host + "'"}, {gs + ".http", "port", port},
			{gs + ".https", "host", "'" + host + "'"}, {gs + ".https", "port", port},
			{gs + ".socks", "host", "'" + host + "'"}, {gs + ".socks", "port", port},
			{gs, "ignore-hosts", liste},
			{gs, "mode", "'manual'"},
		} {
			if err := gset(a[0], a[1], a[2]); err != nil {
				return err
			}
		}
		gesetzt = true
	}
	if kdeWerkzeug("write") != "" {
		url := "http://" + host + " " + port
		if err := kdeSetzen(map[string]string{
			"ProxyType":         "1",
			"httpProxy":         url,
			"httpsProxy":        url,
			"socksProxy":        "socks://" + host + " " + port,
			"NoProxyFor":        strings.Join(ausnahmen, ","),
			"ReversedException": "false",
		}); err != nil {
			return err
		}
		gesetzt = true
	}
	if !gesetzt {
		return keinDesktop()
	}
	return nil
}

// ProxyEntfernen schaltet den Proxy ab, wenn er auf Bondr zeigt.
func ProxyEntfernen(adresse string) error {
	if gnomeAktiv(adresse) {
		gset(gs, "mode", "'none'")
	}
	if kdeAktiv(adresse) {
		kdeSetzen(map[string]string{"ProxyType": "0"})
	}
	return nil
}

// ProxyAktiv sagt, ob der Systemproxy gerade auf adresse zeigt.
func ProxyAktiv(adresse string) bool { return gnomeAktiv(adresse) || kdeAktiv(adresse) }

func gnomeAktiv(adresse string) bool {
	if !gnome() {
		return false
	}
	host, port, _ := strings.Cut(adresse, ":")
	return gget(gs, "mode") == "manual" && gget(gs+".http", "host") == host && gget(gs+".http", "port") == port
}

func kdeAktiv(adresse string) bool {
	if kdeDatei() == "" || kdeWerkzeug("read") == "" {
		return false
	}
	host, port, _ := strings.Cut(adresse, ":")
	return kdeLesen("ProxyType") == "1" && kdeLesen("httpProxy") == "http://"+host+" "+port
}
