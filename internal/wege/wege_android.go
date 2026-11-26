package wege

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"

	"github.com/Dschonas04/bondr/internal/android"
)

// Unter Android meldet die App die Netze (POST /wege); Index ist die Netzkennung.
func finden() ([]Weg, error) { return gesetzt(), nil }

// binden lässt die App den Socket mit Network.bindSocket an das Netz hängen.
func binden(netz uint32) func(string, string, syscall.RawConn) error {
	if netz == 0 {
		return nil
	}
	return func(_, _ string, c syscall.RawConn) error {
		var fehler error
		err := c.Control(func(fd uintptr) { fehler = android.Binden(int(fd), netz) })
		if err != nil {
			return err
		}
		return fehler
	}
}

// aufloeser fragt die Namensserver dieses Netzes, über genau dieses Netz.
func aufloeser(w Weg) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			if len(w.DNS) == 0 {
				return nil, errors.New("Netz " + w.Name + " hat keinen Namensserver")
			}
			var letzter error
			for _, s := range w.DNS {
				d := net.Dialer{Timeout: 4 * time.Second, Control: binden(w.Index)}
				c, err := d.DialContext(ctx, network, net.JoinHostPort(s.String(), "53"))
				if err == nil {
					return c, nil
				}
				letzter = err
			}
			return nil, letzter
		},
	}
}
