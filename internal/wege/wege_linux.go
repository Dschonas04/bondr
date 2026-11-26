//go:build linux && !android

package wege

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func finden() ([]Weg, error) {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gateways := standardGateways(f)
	namen := verbindungsNamen()
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var alle []Weg
	for _, i := range ifs {
		gw, ok := gateways[i.Name]
		if !ok || i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || virtuell(i.Name) {
			continue
		}
		var ip net.IP
		adressen, _ := i.Addrs()
		for _, a := range adressen {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
				ip = n.IP.To4()
				break
			}
		}
		if ip == nil {
			continue
		}
		name := namen[i.Name]
		if name == "" {
			name = i.Name
		}
		alle = append(alle, Weg{
			Schluessel:   i.Name + "|" + ip.String(),
			Name:         name,
			Beschreibung: i.Name,
			Art:          artLinux(i.Name, name),
			Index:        uint32(i.Index),
			IP:           ip,
			Gateway:      gw,
		})
	}
	return alle, nil
}

// virtuell sind Brücken, Container und Tunnel; über sie geht es nicht selbst ins Internet.
func virtuell(dev string) bool {
	for _, p := range []string{"docker", "br-", "veth", "virbr", "vnet", "tun", "tap", "wg", "tailscale", "zt", "lxc", "cni", "flannel", "podman", "vmnet", "vboxnet"} {
		if strings.HasPrefix(dev, p) {
			return true
		}
	}
	return false
}

// standardGateways liest aus /proc/net/route je Adapter die Standardroute mit der
// kleinsten Metrik. NetworkManager legt für jeden verbundenen Adapter eine an.
func standardGateways(r io.Reader) map[string]net.IP {
	type eintrag struct {
		gw     net.IP
		metrik int
	}
	beste := map[string]eintrag{}
	s := bufio.NewScanner(r)
	for s.Scan() {
		f := strings.Fields(s.Text())
		if len(f) < 8 || f[0] == "Iface" || f[1] != "00000000" || f[7] != "00000000" {
			continue
		}
		flags, _ := strconv.ParseUint(f[3], 16, 32)
		if flags&0x1 == 0 || flags&0x2 == 0 { // RTF_UP, RTF_GATEWAY
			continue
		}
		roh, err := hex.DecodeString(f[2])
		if err != nil || len(roh) != 4 {
			continue
		}
		gw := make(net.IP, 4)
		binary.BigEndian.PutUint32(gw, binary.LittleEndian.Uint32(roh))
		metrik, _ := strconv.Atoi(f[6])
		if alt, ok := beste[f[0]]; !ok || metrik < alt.metrik {
			beste[f[0]] = eintrag{gw, metrik}
		}
	}
	out := make(map[string]net.IP, len(beste))
	for k, v := range beste {
		out[k] = v.gw
	}
	return out
}

func artLinux(dev, name string) string {
	if _, err := os.Stat(filepath.Join("/sys/class/net", dev, "wireless")); err == nil {
		return "WLAN"
	}
	if treiber := treiberName(dev); enthaelt(treiber, "rndis", "cdc_ether", "cdc_ncm", "ipheth", "qmi_wwan", "cdc_mbim") {
		return "Handy"
	}
	switch {
	case strings.HasPrefix(dev, "usb"), strings.HasPrefix(dev, "wwan"), strings.HasPrefix(dev, "ww"), enthaelt(name, "Android", "iPhone", "Handy", "Mobile", "Tethering"):
		return "Handy"
	case strings.HasPrefix(dev, "wl"):
		return "WLAN"
	case strings.HasPrefix(dev, "en"), strings.HasPrefix(dev, "eth"):
		return "Kabel"
	}
	return "Sonstiges"
}

func treiberName(dev string) string {
	z, err := os.Readlink(filepath.Join("/sys/class/net", dev, "device", "driver"))
	if err != nil {
		return ""
	}
	return filepath.Base(z)
}

var (
	namenMu    sync.Mutex
	namenCache map[string]string
	namenZeit  time.Time
)

// verbindungsNamen ordnet Gerätenamen (wlp2s0) den Namen aus NetworkManager zu
// (FRITZ!Box 7590, Kabelgebundene Verbindung 1). Eine Minute zwischengespeichert.
func verbindungsNamen() map[string]string {
	namenMu.Lock()
	defer namenMu.Unlock()
	if namenCache != nil && time.Since(namenZeit) < time.Minute {
		return namenCache
	}
	out := map[string]string{}
	if b, err := exec.Command("nmcli", "-t", "-f", "DEVICE,CONNECTION", "device").Output(); err == nil {
		for _, z := range strings.Split(string(b), "\n") {
			dev, verb, ok := strings.Cut(z, ":")
			if ok && dev != "" && verb != "" && verb != "--" {
				out[dev] = strings.ReplaceAll(verb, `\:`, ":")
			}
		}
	}
	namenCache, namenZeit = out, time.Now()
	return out
}

// binden setzt SO_BINDTODEVICE, damit Linux die Verbindung über genau diesen Adapter
// schickt. Seit Linux 5.7 geht das ohne Rootrechte.
func binden(index uint32) func(string, string, syscall.RawConn) error {
	if index == 0 {
		return nil // ohne Adapter, etwa in Tests
	}
	return func(_, _ string, c syscall.RawConn) error {
		i, err := net.InterfaceByIndex(int(index))
		if err != nil {
			return err
		}
		var fehler error
		err = c.Control(func(fd uintptr) {
			fehler = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, i.Name)
		})
		if err != nil {
			return err
		}
		return fehler
	}
}

func aufloeser(Weg) *net.Resolver { return net.DefaultResolver }
