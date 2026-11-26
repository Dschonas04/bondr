//go:build tunnelecht

// Echter Test mit TUN-Gerät (root): go test -tags tunnelecht ./internal/tunnel
// Leitet nur 1.0.0.1/32 und 172.19.0.2/32 in den Tunnel, der Rest des Rechners bleibt unberührt.
package tunnel

import (
	"context"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/Dschonas04/bondr/internal/verteiler"
	"github.com/Dschonas04/bondr/internal/wege"
)

func tunOeffnen(t *testing.T, name string) int {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	var ifr [unix.IFNAMSIZ + 64]byte
	copy(ifr[:], name)
	*(*uint16)(unsafe.Pointer(&ifr[unix.IFNAMSIZ])) = unix.IFF_TUN | unix.IFF_NO_PI
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TUNSETIFF, uintptr(unsafe.Pointer(&ifr[0]))); e != 0 {
		t.Fatal(e)
	}
	return fd
}

func ip(t *testing.T, a ...string) {
	if b, err := exec.Command("ip", a...).CombinedOutput(); err != nil {
		t.Fatalf("ip %v: %v %s", a, err, b)
	}
}

func TestEcht(t *testing.T) {
	fd := tunOeffnen(t, "bondrt0")
	ip(t, "addr", "add", "172.19.0.1/30", "dev", "bondrt0")
	ip(t, "link", "set", "bondrt0", "up")
	ip(t, "route", "add", "1.0.0.1/32", "dev", "bondrt0")
	ip(t, "route", "add", "172.19.0.2/32", "dev", "bondrt0")
	defer exec.Command("ip", "link", "del", "bondrt0").Run()

	v := verteiler.NeuMit(func() ([]wege.Weg, error) {
		w, err := wege.Finden()
		for i := range w {
			w[i].DNS = []net.IP{net.IPv4(1, 1, 1, 1).To4()}
		}
		return w, err
	}, nil)
	ctx, stopp := context.WithCancel(context.Background())
	defer stopp()
	go Laufen(ctx, fd, v)
	time.Sleep(500 * time.Millisecond)

	out, err := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "10", "https://1.0.0.1/").CombinedOutput()
	t.Logf("TCP über Tunnel: %s %v", out, err)
	if string(out) != "301" && string(out) != "200" {
		t.Errorf("TCP ging nicht durch: %s", out)
	}
	out, err = exec.Command("dig", "+short", "+time=5", "+tries=1", "@172.19.0.2", "example.com").CombinedOutput()
	t.Logf("DNS über Tunnel: %s %v", out, err)
	if len(out) < 7 {
		t.Errorf("DNS ging nicht durch")
	}
	out, err = exec.Command("bash", "-c", `exec 3<>/dev/tcp/1.0.0.1/80; printf 'GET' >&3; sleep 0.3; printf ' / HTTP/1.0\r\nHost: one.one.one.one\r\n\r\n' >&3; timeout 5 head -1 <&3`).CombinedOutput()
	t.Logf("TCP in Stücken: %q %v", out, err)
	if len(out) < 8 {
		t.Errorf("Antwort fehlt bei mehreren Segmenten")
	}
	for _, s := range v.Stand() {
		t.Logf("Weg %s: %.3f MB rein", s.Name, s.MBRein)
	}
	_ = os.Stdout
}
