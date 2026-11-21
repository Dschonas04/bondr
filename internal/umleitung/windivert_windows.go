package umleitung

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// adresse entspricht WINDIVERT_ADDRESS (80 Byte).
type adresse struct {
	Zeit   int64
	Bits   uint32
	_      uint32
	IfIdx  uint32
	SubIdx uint32
	_      [56]byte
}

const (
	bitAusgehend = 1 << 17
	mtuMax       = 40 + 0xFFFF
)

type divert struct {
	h                                    windows.Handle
	recv, send, close, shutdown, pruefen *windows.LazyProc
}

func divertOeffnen(filter string, prioritaet int16) (*divert, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dll := windows.NewLazyDLL(filepath.Join(filepath.Dir(exe), "WinDivert.dll"))
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("WinDivert.dll fehlt neben bondrd.exe: %w", err)
	}
	d := &divert{
		recv:     dll.NewProc("WinDivertRecv"),
		send:     dll.NewProc("WinDivertSend"),
		close:    dll.NewProc("WinDivertClose"),
		shutdown: dll.NewProc("WinDivertShutdown"),
		pruefen:  dll.NewProc("WinDivertHelperCalcChecksums"),
	}
	f, err := windows.BytePtrFromString(filter)
	if err != nil {
		return nil, err
	}
	r, _, e := dll.NewProc("WinDivertOpen").Call(uintptr(unsafe.Pointer(f)), 0, uintptr(prioritaet), 0)
	if windows.Handle(r) == windows.InvalidHandle {
		return nil, fmt.Errorf("WinDivert öffnen: %w", e)
	}
	d.h = windows.Handle(r)
	return d, nil
}

func (d *divert) empfangen(puffer []byte, a *adresse) (int, error) {
	var n uint32
	r, _, e := d.recv.Call(uintptr(d.h), uintptr(unsafe.Pointer(&puffer[0])), uintptr(len(puffer)), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(a)))
	if r == 0 {
		return 0, e
	}
	return int(n), nil
}

func (d *divert) senden(paket []byte, a *adresse) error {
	var n uint32
	r, _, e := d.send.Call(uintptr(d.h), uintptr(unsafe.Pointer(&paket[0])), uintptr(len(paket)), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(a)))
	if r == 0 {
		return e
	}
	return nil
}

func (d *divert) pruefsummen(paket []byte, a *adresse) {
	d.pruefen.Call(uintptr(unsafe.Pointer(&paket[0])), uintptr(len(paket)), uintptr(unsafe.Pointer(a)), 0)
}

// beenden lässt laufende Empfänge mit Fehler zurückkehren und schließt danach.
func (d *divert) beenden() {
	d.shutdown.Call(uintptr(d.h), 3) // WINDIVERT_SHUTDOWN_BOTH
	d.close.Call(uintptr(d.h))
}
