// Paket tunnel nimmt unter Android den gesamten Verkehr aus dem VPN-Tunnel entgegen
// (Netzwerkstapel von gVisor über tun2socks) und verteilt jede TCP-Verbindung und
// jeden UDP-Fluss über den Lastverteiler auf WLAN und Mobilfunk.
package tunnel

import (
	"context"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/core"
	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
	"github.com/xjasonlyu/tun2socks/v2/core/device/fdbased"

	"github.com/Dschonas04/bondr/internal/verteiler"
)

// DNS ist die Adresse des Namensservers im Tunnel; Anfragen dorthin beantwortet der
// Namensserver des Netzes, über das sie gerade gehen.
var DNS = netip.MustParseAddr("172.19.0.2")

// MTU des Tunnels, muss zur App passen.
const MTU = 1500

type verteilen struct {
	ctx context.Context
	v   *verteiler.Verteiler
}

// Laufen betreibt den Tunnel auf dem Dateideskriptor fd, bis ctx endet.
func Laufen(ctx context.Context, fd int, v *verteiler.Verteiler) error {
	dev, err := fdbased.Open(itoa(fd), MTU, 0)
	if err != nil {
		return err
	}
	s, err := core.CreateStack(&core.Config{
		LinkEndpoint:     dev,
		TransportHandler: &verteilen{ctx: ctx, v: v},
	})
	if err != nil {
		dev.Close()
		return err
	}
	log.Printf("Tunnel läuft auf fd %d", fd)
	<-ctx.Done()
	s.Close()
	dev.Close()
	return nil
}

func itoa(i int) string {
	b := []byte{}
	if i == 0 {
		return "0"
	}
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

// HandleTCP verbindet nach außen über den Weg, den der Verteiler wählt.
func (t *verteilen) HandleTCP(c adapter.TCPConn) {
	defer c.Close()
	id := c.ID()
	adr, ok := netip.AddrFromSlice(id.LocalAddress.AsSlice())
	if !ok {
		return
	}
	adr = adr.Unmap()
	if adr == DNS {
		if id.LocalPort != 53 {
			return // DNS über TLS (853) gibt es hier nicht, Android nimmt dann Port 53
		}
		adr = netip.Addr{} // TCP-DNS: an den Namensserver des Wegs
	}
	ctx, fertig := context.WithTimeout(t.ctx, 15*time.Second)
	var z net.Conn
	var err error
	ziel := netip.AddrPortFrom(adr, id.LocalPort).String()
	if !adr.IsValid() {
		z, err = t.dnsTCP(ctx)
	} else {
		z, _, err = t.v.Waehlen(ctx, ziel)
	}
	fertig()
	if err != nil {
		protokoll("TCP %s: %v", ziel, err)
		return
	}
	defer z.Close()
	var wg sync.WaitGroup
	var raus, rein int64
	wg.Add(2)
	go func() { defer wg.Done(); raus, _ = io.Copy(z, c); halbZu(z) }()
	go func() { defer wg.Done(); rein, _ = io.Copy(c, z); halbZu(c) }()
	wg.Wait()
	protokoll("TCP %s: %d raus, %d rein", ziel, raus, rein)
}

// Ausführlich ist nur zum Prüfen gedacht (BONDR_PROTOKOLL=1).
var ausfuehrlich = os.Getenv("BONDR_PROTOKOLL") != ""

func protokoll(f string, a ...any) {
	if ausfuehrlich {
		log.Printf(f, a...)
	}
}

func halbZu(c net.Conn) {
	if h, ok := c.(interface{ CloseWrite() error }); ok {
		h.CloseWrite()
	} else {
		c.Close()
	}
}

func (t *verteilen) dnsTCP(ctx context.Context) (net.Conn, error) {
	p, err := t.v.Naechster(false)
	if err != nil {
		return nil, err
	}
	d := p.DNS4()
	if d == nil {
		d = net.IPv4(1, 1, 1, 1)
	}
	return t.v.WaehlenUeber(ctx, p, net.JoinHostPort(d.String(), "53"))
}

// HandleUDP leitet einen UDP-Fluss (DNS, QUIC, Spiele) über einen gewählten Weg.
func (t *verteilen) HandleUDP(c adapter.UDPConn) {
	defer c.Close()
	id := c.ID()
	adr, ok := netip.AddrFromSlice(id.LocalAddress.AsSlice())
	if !ok {
		return
	}
	adr = adr.Unmap()
	v6 := adr.Is6()
	p, err := t.v.Naechster(v6)
	if err != nil {
		return
	}
	ziel := &net.UDPAddr{IP: adr.AsSlice(), Port: int(id.LocalPort)}
	warten := 2 * time.Minute
	if adr == DNS {
		d := p.DNS4()
		if d == nil {
			d = net.IPv4(1, 1, 1, 1)
		}
		ziel = &net.UDPAddr{IP: d, Port: 53}
		warten = 15 * time.Second
	}
	pc, err := t.v.PaketZaehlen(t.ctx, p, v6)
	if err != nil {
		return
	}
	defer pc.Close()
	go func() {
		b := make([]byte, 64*1024)
		for {
			pc.SetReadDeadline(time.Now().Add(warten))
			n, _, err := pc.ReadFrom(b)
			if err != nil {
				c.Close()
				return
			}
			if _, err := c.Write(b[:n]); err != nil {
				return
			}
		}
	}()
	b := make([]byte, 64*1024)
	for {
		c.SetReadDeadline(time.Now().Add(warten))
		n, err := c.Read(b)
		if err != nil {
			return
		}
		if _, err := pc.WriteTo(b[:n], ziel); err != nil {
			return
		}
	}
}
