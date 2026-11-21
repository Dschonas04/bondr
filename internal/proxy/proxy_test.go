package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Dschonas04/bondr/internal/verteiler"
	"github.com/Dschonas04/bondr/internal/wege"
)

func starten(t *testing.T) (string, *verteiler.Verteiler) {
	v := verteiler.NeuMit(func() ([]wege.Weg, error) {
		return []wege.Weg{
			{Schluessel: "a", Name: "A", IP: net.IPv4(127, 0, 0, 1)},
			{Schluessel: "b", Name: "B", IP: net.IPv4(127, 0, 0, 1)},
		}, nil
	})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stopp := context.WithCancel(context.Background())
	t.Cleanup(stopp)
	go (&Proxy{V: v}).Dienen(ctx, l)
	return l.Addr().String(), v
}

func ziel(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok "+r.URL.Path)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestHTTPProxyVerteilt(t *testing.T) {
	adr, v := starten(t)
	s := ziel(t)
	pu, _ := url.Parse("http://" + adr)
	c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu), DisableKeepAlives: true}}
	for i := 0; i < 10; i++ {
		r, err := c.Get(s.URL + "/x")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if string(b) != "ok /x" {
			t.Fatalf("Antwort %q", b)
		}
	}
	for _, st := range v.Stand() {
		if st.GBRein == 0 {
			t.Errorf("Weg %s nie benutzt", st.Name)
		}
	}
}

func TestConnectUndSocks(t *testing.T) {
	adr, _ := starten(t)
	s := ziel(t)
	host := strings.TrimPrefix(s.URL, "http://")

	// HTTP CONNECT
	c, err := net.Dial("tcp", adr)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", host, host)
	r := bufio.NewReader(c)
	zeile, _ := r.ReadString('\n')
	if !strings.Contains(zeile, "200") {
		t.Fatalf("CONNECT: %q", zeile)
	}
	r.ReadString('\n')
	fmt.Fprintf(c, "GET /tunnel HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", host)
	b, _ := io.ReadAll(r)
	if !strings.Contains(string(b), "ok /tunnel") {
		t.Fatalf("Tunnel: %q", b)
	}
	c.Close()

	// SOCKS5
	pu, _ := url.Parse("socks5://" + adr)
	hc := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
	resp, err := hc.Get(s.URL + "/socks")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "ok /socks" {
		t.Fatalf("SOCKS: %q", b)
	}
}
