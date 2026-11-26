//go:build linux && !android

package wege

import (
	"strings"
	"testing"
)

func TestStandardGateways(t *testing.T) {
	tabelle := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
enp3s0	00000000	0100000A	0003	0	0	100	00000000	0	0	0
wlp2s0	00000000	01B2A8C0	0003	0	0	600	00000000	0	0	0
wlp2s0	00000000	FEB2A8C0	0003	0	0	900	00000000	0	0	0
enp3s0	0000000A	00000000	0001	0	0	100	000000FF	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
`
	g := standardGateways(strings.NewReader(tabelle))
	if len(g) != 2 {
		t.Fatalf("zwei Adapter erwartet, %v", g)
	}
	if g["enp3s0"].String() != "10.0.0.1" {
		t.Errorf("enp3s0: %v", g["enp3s0"])
	}
	if g["wlp2s0"].String() != "192.168.178.1" {
		t.Errorf("wlp2s0 mit kleinster Metrik: %v", g["wlp2s0"])
	}
}

func TestVirtuell(t *testing.T) {
	for _, d := range []string{"docker0", "br-1a2b", "veth12", "virbr0", "tailscale0", "wg0"} {
		if !virtuell(d) {
			t.Errorf("%s gilt nicht als virtuell", d)
		}
	}
	for _, d := range []string{"enp3s0", "wlp2s0", "usb0", "enx0a1b2c3d4e5f"} {
		if virtuell(d) {
			t.Errorf("%s gilt als virtuell", d)
		}
	}
}
