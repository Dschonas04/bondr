//go:build linux

package android

import (
	"net"
	"os"
	"testing"
)

func TestEigenerBenutzer(t *testing.T) {
	Name = "@bondr-test-" + string(rune('a'+os.Getpid()%26))
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: Name, Net: "unix"})
	if err != nil {
		t.Skip(err)
	}
	defer l.Close()
	go func() {
		c, err := l.Accept()
		if err == nil {
			c.Close()
		}
	}()
	c, err := verbinden()
	if err != nil {
		t.Fatalf("eigener Benutzer abgelehnt: %v", err)
	}
	c.Close()
	verb = nil
}
