//go:build unix

// Paket android spricht mit der Android-App über einen Unix-Socket im abstrakten
// Namensraum. Nur die App darf Sockets an ein Netz (WLAN, Mobilfunk) binden
// (Network.bindSocket) und den VPN-Tunnel öffnen; der Kern schickt ihr dafür den
// Dateideskriptor und bekommt den Tunnel als Dateideskriptor zurück.
package android

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"
)

// Name des Sockets der App, "@" steht für den abstrakten Namensraum.
var Name = "@bondr-app"

const (
	befehlBinden = 'B'
	befehlTunnel = 'T'
)

var (
	mu   sync.Mutex
	verb *net.UnixConn
)

func verbinden() (*net.UnixConn, error) {
	if verb != nil {
		return verb, nil
	}
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: Name, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("App nicht erreichbar: %w", err)
	}
	verb = c
	return c, nil
}

// anfrage schickt einen Befehl (optional mit Deskriptor) und liest die Antwort:
// ein Byte Ergebnis (0 = gut) und, falls die App einen schickt, einen Deskriptor.
func anfrage(befehl byte, wert uint32, fd int) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	for versuch := 0; versuch < 2; versuch++ {
		c, err := verbinden()
		if err != nil {
			return -1, err
		}
		b := make([]byte, 5)
		b[0] = befehl
		binary.BigEndian.PutUint32(b[1:], wert)
		var oob []byte
		if fd >= 0 {
			oob = syscall.UnixRights(fd)
		}
		c.SetDeadline(time.Now().Add(10 * time.Second))
		if _, _, err := c.WriteMsgUnix(b, oob, nil); err != nil {
			c.Close()
			verb = nil
			continue
		}
		antwort := make([]byte, 1)
		rueck := make([]byte, syscall.CmsgSpace(4))
		n, oobn, _, _, err := c.ReadMsgUnix(antwort, rueck)
		if err != nil || n != 1 {
			c.Close()
			verb = nil
			if err == nil {
				err = errors.New("leere Antwort")
			}
			return -1, fmt.Errorf("App antwortet nicht: %w", err)
		}
		if antwort[0] != 0 {
			return -1, fmt.Errorf("App lehnt ab (%d)", antwort[0])
		}
		if oobn > 0 {
			msgs, err := syscall.ParseSocketControlMessage(rueck[:oobn])
			if err == nil && len(msgs) > 0 {
				if fds, err := syscall.ParseUnixRights(&msgs[0]); err == nil && len(fds) > 0 {
					return fds[0], nil
				}
			}
		}
		return -1, nil
	}
	return -1, errors.New("App nicht erreichbar")
}

// Binden bindet den Socket fd an das Netz mit der Kennung netz (Network.toString()).
func Binden(fd int, netz uint32) error {
	_, err := anfrage(befehlBinden, netz, fd)
	return err
}

// Tunnel holt den Dateideskriptor des VPN-Tunnels.
func Tunnel() (int, error) {
	fd, err := anfrage(befehlTunnel, 0, -1)
	if err == nil && fd < 0 {
		err = errors.New("App hat keinen Tunnel geschickt")
	}
	return fd, err
}
