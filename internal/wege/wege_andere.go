//go:build !windows

package wege

import (
	"net"
	"syscall"
)

// Außerhalb von Windows nur zum Entwickeln: alle Adapter mit IPv4, ohne Gateway-Prüfung.
func finden() ([]Weg, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var alle []Weg
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		adressen, _ := i.Addrs()
		for _, a := range adressen {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil || n.IP.IsLinkLocalUnicast() {
				continue
			}
			alle = append(alle, Weg{
				Schluessel: i.Name + "|" + n.IP.String(),
				Name:       i.Name,
				Art:        artAusName(i.Name, ""),
				Index:      uint32(i.Index),
				IP:         n.IP.To4(),
			})
			break
		}
	}
	return alle, nil
}

func binden(uint32) func(string, string, syscall.RawConn) error { return nil }
