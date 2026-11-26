package wege

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	gaaFlagIncludeGateways = 0x0080
	gaaFlagSkipAnycast     = 0x0002
	gaaFlagSkipMulticast   = 0x0004
	gaaFlagSkipDNSServer   = 0x0008
	ifOperStatusUp         = 1
	ifTypeSoftwareLoopback = 24
	ipUnicastIf            = 31 // IP_UNICAST_IF
)

func finden() ([]Weg, error) {
	groesse := uint32(16 * 1024)
	var puffer []byte
	for versuch := 0; versuch < 4; versuch++ {
		puffer = make([]byte, groesse)
		err := windows.GetAdaptersAddresses(windows.AF_INET,
			gaaFlagIncludeGateways|gaaFlagSkipAnycast|gaaFlagSkipMulticast|gaaFlagSkipDNSServer,
			0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&puffer[0])), &groesse)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil, fmt.Errorf("Adapter abfragen: %w", err)
		}
	}
	var alle []Weg
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&puffer[0])); a != nil; a = a.Next {
		if a.OperStatus != ifOperStatusUp || a.IfType == ifTypeSoftwareLoopback || a.FirstGatewayAddress == nil {
			continue
		}
		var ip, gw net.IP
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			if v := sockIP(u.Address); v != nil && !v.IsLinkLocalUnicast() {
				ip = v
				break
			}
		}
		for g := a.FirstGatewayAddress; g != nil; g = g.Next {
			if v := sockIP(g.Address); v != nil && !v.IsUnspecified() {
				gw = v
				break
			}
		}
		if ip == nil || gw == nil {
			continue
		}
		name := windows.UTF16PtrToString(a.FriendlyName)
		beschr := windows.UTF16PtrToString(a.Description)
		alle = append(alle, Weg{
			Schluessel:   name + "|" + ip.String(),
			Name:         name,
			Beschreibung: beschr,
			Art:          artAusName(name, beschr),
			Index:        a.IfIndex,
			IP:           ip,
			Gateway:      gw,
		})
	}
	return alle, nil
}

func sockIP(s windows.SocketAddress) net.IP {
	if s.Sockaddr == nil {
		return nil
	}
	return s.IP().To4()
}

// binden setzt IP_UNICAST_IF. Damit nimmt Windows den Adapter auch dann, wenn die
// Routentabelle einen anderen bevorzugt. Der Index muss in Netzwerk-Byte-Reihenfolge stehen.
func binden(index uint32) func(string, string, syscall.RawConn) error {
	return func(_, _ string, c syscall.RawConn) error {
		var fehler error
		err := c.Control(func(fd uintptr) {
			var b [4]byte
			binary.BigEndian.PutUint32(b[:], index)
			wert := int(binary.LittleEndian.Uint32(b[:]))
			fehler = windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, ipUnicastIf, wert)
		})
		if err != nil {
			return err
		}
		return fehler
	}
}

func aufloeser(Weg) *net.Resolver { return net.DefaultResolver }
