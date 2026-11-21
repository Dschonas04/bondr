package system

import (
	"fmt"
	"os/exec"
	"strings"
)

const ns = "/usr/sbin/networksetup"

var ausnahmen = []string{"localhost", "127.0.0.1", "*.local", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "*.fritz.box"}

// dienste liefert die eingeschalteten Netzwerkdienste (WLAN, Ethernet, iPhone USB …).
func dienste() ([]string, error) {
	b, err := exec.Command(ns, "-listallnetworkservices").Output()
	if err != nil {
		return nil, err
	}
	var out []string
	for i, z := range strings.Split(string(b), "\n") {
		z = strings.TrimSpace(z)
		if i == 0 || z == "" || strings.HasPrefix(z, "*") {
			continue
		}
		out = append(out, z)
	}
	return out, nil
}

func ausfuehren(args ...string) error {
	if b, err := exec.Command(ns, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("networksetup %s: %v %s", args[0], err, strings.TrimSpace(string(b)))
	}
	return nil
}

// ProxySetzen trägt Bondr als HTTP- und HTTPS-Proxy in allen Netzwerkdiensten ein.
func ProxySetzen(adresse string) error {
	host, port, _ := strings.Cut(adresse, ":")
	l, err := dienste()
	if err != nil {
		return err
	}
	for _, d := range l {
		for _, a := range [][]string{
			{"-setwebproxy", d, host, port},
			{"-setsecurewebproxy", d, host, port},
			append([]string{"-setproxybypassdomains", d}, ausnahmen...),
		} {
			if err := ausfuehren(a...); err != nil {
				return err
			}
		}
	}
	return nil
}

// ProxyEntfernen schaltet den Proxy in allen Diensten ab, in denen er auf Bondr zeigt.
func ProxyEntfernen(adresse string) error {
	host, port, _ := strings.Cut(adresse, ":")
	l, err := dienste()
	if err != nil {
		return err
	}
	for _, d := range l {
		b, _ := exec.Command(ns, "-getwebproxy", d).Output()
		t := string(b)
		if !strings.Contains(t, "Server: "+host) || !strings.Contains(t, "Port: "+port) {
			continue
		}
		ausfuehren("-setwebproxystate", d, "off")
		ausfuehren("-setsecurewebproxystate", d, "off")
	}
	return nil
}

// ProxyAktiv sagt, ob der Systemproxy gerade auf adresse zeigt.
func ProxyAktiv(adresse string) bool {
	host, port, _ := strings.Cut(adresse, ":")
	b, err := exec.Command("/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return false
	}
	t := string(b)
	return strings.Contains(t, "HTTPEnable : 1") && strings.Contains(t, "HTTPProxy : "+host) && strings.Contains(t, "HTTPPort : "+port)
}
