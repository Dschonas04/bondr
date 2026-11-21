// Paket proxy nimmt Verbindungen als SOCKS5- und HTTP-Proxy auf demselben Port an
// und gibt jede an den Verteiler weiter.
package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Dschonas04/bondr/internal/verteiler"
)

// Proxy lauscht auf einer lokalen Adresse.
type Proxy struct {
	V *verteiler.Verteiler
}

// Dienen nimmt Verbindungen an, bis ctx endet.
func (p *Proxy) Dienen(ctx context.Context, l net.Listener) error {
	go func() { <-ctx.Done(); l.Close() }()
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		go p.bedienen(ctx, c)
	}
}

func (p *Proxy) bedienen(ctx context.Context, c net.Conn) {
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(30 * time.Second))
	r := bufio.NewReader(c)
	erstes, err := r.Peek(1)
	if err != nil {
		return
	}
	if erstes[0] == 0x05 {
		p.socks(ctx, c, r)
		return
	}
	p.http(ctx, c, r)
}

// socks5 nach RFC 1928, nur CONNECT ohne Anmeldung.
func (p *Proxy) socks(ctx context.Context, c net.Conn, r *bufio.Reader) {
	kopf := make([]byte, 2)
	if _, err := io.ReadFull(r, kopf); err != nil {
		return
	}
	methoden := make([]byte, kopf[1])
	if _, err := io.ReadFull(r, methoden); err != nil {
		return
	}
	c.Write([]byte{5, 0})
	anfrage := make([]byte, 4)
	if _, err := io.ReadFull(r, anfrage); err != nil || anfrage[0] != 5 {
		return
	}
	if anfrage[1] != 1 { // nur CONNECT
		c.Write([]byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	var host string
	switch anfrage[3] {
	case 1:
		b := make([]byte, 4)
		if _, err := io.ReadFull(r, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case 3:
		l, err := r.ReadByte()
		if err != nil {
			return
		}
		b := make([]byte, l)
		if _, err := io.ReadFull(r, b); err != nil {
			return
		}
		host = string(b)
	case 4:
		b := make([]byte, 16)
		if _, err := io.ReadFull(r, b); err != nil {
			return
		}
		host = net.IP(b).String()
	default:
		c.Write([]byte{5, 8, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(r, pb); err != nil {
		return
	}
	ziel := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb))))
	z, _, err := p.V.Waehlen(ctx, ziel)
	if err != nil {
		c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer z.Close()
	c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	c.SetReadDeadline(time.Time{})
	durchreichen(c, r, z)
}

func (p *Proxy) http(ctx context.Context, c net.Conn, r *bufio.Reader) {
	for {
		req, err := http.ReadRequest(r)
		if err != nil {
			return
		}
		c.SetReadDeadline(time.Time{})
		if req.Method == http.MethodConnect {
			z, _, err := p.V.Waehlen(ctx, mitPort(req.Host, "443"))
			if err != nil {
				log.Printf("CONNECT %s: %v", req.Host, err)
				fmt.Fprintf(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
				return
			}
			defer z.Close()
			fmt.Fprintf(c, "HTTP/1.1 200 Connection established\r\n\r\n")
			durchreichen(c, r, z)
			return
		}
		// Normales HTTP: jede Anfrage ist eine eigene Verbindung nach außen, damit sich
		// auch unverschlüsselte Abrufe verteilen.
		if req.URL.Host == "" {
			fmt.Fprintf(c, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
			return
		}
		weiter := p.einfach(ctx, c, req)
		if !weiter {
			return
		}
	}
}

func (p *Proxy) einfach(ctx context.Context, c net.Conn, req *http.Request) bool {
	z, weg, err := p.V.Waehlen(ctx, mitPort(req.URL.Host, "80"))
	if err != nil {
		log.Printf("HTTP %s: %v", req.URL.Host, err)
		fmt.Fprintf(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return false
	}
	defer z.Close()
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Proxy-Authorization")
	req.RequestURI = ""
	kundeSchliesst := req.Close
	req.Close = true
	if err := req.Write(z); err != nil {
		return false
	}
	antwort, err := http.ReadResponse(bufio.NewReader(z), req)
	if err != nil {
		log.Printf("HTTP %s über %s: %v", req.URL.Host, weg.Name, err)
		fmt.Fprintf(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return false
	}
	defer antwort.Body.Close()
	antwort.Close = kundeSchliesst
	behalten := !kundeSchliesst
	if err := antwort.Write(c); err != nil {
		return false
	}
	return behalten
}

func mitPort(host, standard string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(strings.Trim(host, "[]"), standard)
}

// durchreichen kopiert in beide Richtungen, bis eine Seite fertig ist.
func durchreichen(c net.Conn, r *bufio.Reader, z net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(z, r)
		halbZu(z)
	}()
	go func() {
		defer wg.Done()
		io.Copy(c, z)
		halbZu(c)
	}()
	wg.Wait()
}

func halbZu(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
		return
	}
	c.Close()
}
