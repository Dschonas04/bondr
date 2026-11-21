package wege

import (
	"bufio"
	"bytes"
	"net"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/route"
)

const ipBoundIf = 25 // IP_BOUND_IF aus netinet/in.h

func finden() ([]Weg, error) {
	gateways, err := standardGateways()
	if err != nil {
		return nil, err
	}
	ports := hardwarePorts()
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var alle []Weg
	for _, i := range ifs {
		gw, ok := gateways[i.Index]
		if !ok || i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		if strings.HasPrefix(i.Name, "utun") || strings.HasPrefix(i.Name, "ipsec") || strings.HasPrefix(i.Name, "bridge") {
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
		name := ports[i.Name]
		if name == "" {
			name = i.Name
		}
		alle = append(alle, Weg{
			Schluessel:   i.Name + "|" + ip.String(),
			Name:         name,
			Beschreibung: i.Name,
			Art:          artMac(name, i.Name),
			Index:        uint32(i.Index),
			IP:           ip,
			Gateway:      gw,
		})
	}
	return alle, nil
}

func artMac(port, dev string) string {
	switch {
	case enthaelt(port, "iPhone", "iPad", "Android", "RNDIS", "Mobile"):
		return "Handy"
	case enthaelt(port, "Wi-Fi", "WLAN", "AirPort"):
		return "WLAN"
	case enthaelt(port, "Ethernet", "LAN", "Thunderbolt", "USB"):
		return "Kabel"
	}
	return "Sonstiges"
}

// standardGateways liest aus der Routentabelle je Adapter die Standardroute. macOS
// führt für jeden Adapter eine eigene (scoped) Standardroute, auch wenn er nicht der
// bevorzugte ist.
func standardGateways() (map[int]net.IP, error) {
	rib, err := route.FetchRIB(syscall.AF_INET, route.RIBTypeRoute, 0)
	if err != nil {
		return nil, err
	}
	msgs, err := route.ParseRIB(route.RIBTypeRoute, rib)
	if err != nil {
		return nil, err
	}
	out := map[int]net.IP{}
	for _, m := range msgs {
		rm, ok := m.(*route.RouteMessage)
		if !ok || len(rm.Addrs) <= syscall.RTAX_NETMASK {
			continue
		}
		dst, ok := rm.Addrs[syscall.RTAX_DST].(*route.Inet4Addr)
		if !ok || dst.IP != [4]byte{} {
			continue
		}
		if mk, ok := rm.Addrs[syscall.RTAX_NETMASK].(*route.Inet4Addr); ok && mk.IP != [4]byte{} {
			continue
		}
		gw, ok := rm.Addrs[syscall.RTAX_GATEWAY].(*route.Inet4Addr)
		if !ok || gw.IP == [4]byte{} {
			continue
		}
		if _, schon := out[rm.Index]; !schon {
			out[rm.Index] = net.IPv4(gw.IP[0], gw.IP[1], gw.IP[2], gw.IP[3]).To4()
		}
	}
	return out, nil
}

var (
	portsMu    sync.Mutex
	portsCache map[string]string
	portsZeit  time.Time
)

// hardwarePorts ordnet Gerätenamen (en0) den Namen aus den Systemeinstellungen zu
// (WLAN, Ethernet, iPhone USB). Eine Minute zwischengespeichert.
func hardwarePorts() map[string]string {
	portsMu.Lock()
	defer portsMu.Unlock()
	if portsCache != nil && time.Since(portsZeit) < time.Minute {
		return portsCache
	}
	out := map[string]string{}
	b, err := exec.Command("/usr/sbin/networksetup", "-listallhardwareports").Output()
	if err == nil {
		var port string
		s := bufio.NewScanner(bytes.NewReader(b))
		for s.Scan() {
			z := s.Text()
			if v, ok := strings.CutPrefix(z, "Hardware Port: "); ok {
				port = v
			} else if v, ok := strings.CutPrefix(z, "Device: "); ok && port != "" {
				out[v] = port
			}
		}
	}
	portsCache, portsZeit = out, time.Now()
	return out
}

// binden setzt IP_BOUND_IF, damit macOS die Verbindung über genau diesen Adapter
// schickt, auch wenn ein anderer der bevorzugte ist.
func binden(index uint32) func(string, string, syscall.RawConn) error {
	return func(_, _ string, c syscall.RawConn) error {
		var fehler error
		err := c.Control(func(fd uintptr) {
			fehler = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, ipBoundIf, int(index))
		})
		if err != nil {
			return err
		}
		return fehler
	}
}
