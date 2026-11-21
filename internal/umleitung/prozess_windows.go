package umleitung

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iphlpapi            = windows.NewLazySystemDLL("iphlpapi.dll")
	getExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
)

const tcpTableOwnerPidAll = 5

// besitzer findet den Prozess, dem die Verbindung lokalPort → ziel:zielPort gehört. Beim
// ersten SYN steht sie schon als SYN_SENT in der Tabelle.
func besitzer(lokalPort uint16, ziel [4]byte, zielPort uint16) uint32 {
	groesse := uint32(64 * 1024)
	for versuch := 0; versuch < 3; versuch++ {
		puffer := make([]byte, groesse)
		r, _, _ := getExtendedTcpTable.Call(uintptr(unsafe.Pointer(&puffer[0])), uintptr(unsafe.Pointer(&groesse)), 0, windows.AF_INET, tcpTableOwnerPidAll, 0)
		if r == uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			groesse += 16 * 1024
			continue
		}
		if r != 0 {
			return 0
		}
		anzahl := int(binary.LittleEndian.Uint32(puffer[0:4]))
		for i := 0; i < anzahl; i++ {
			z := puffer[4+i*24:]
			if len(z) < 24 {
				break
			}
			lp := binary.BigEndian.Uint16(z[8:10]) // dwLocalPort in Netzreihenfolge
			rp := binary.BigEndian.Uint16(z[16:18])
			if lp == lokalPort && rp == zielPort && [4]byte(z[12:16]) == ziel {
				return binary.LittleEndian.Uint32(z[20:24])
			}
		}
		return 0
	}
	return 0
}

type namensSpeicher struct {
	mu sync.Mutex
	m  map[uint32]namenEintrag
}

type namenEintrag struct {
	name string
	bis  time.Time
}

// name liefert den Dateinamen des Prozesses in Kleinbuchstaben, zwischengespeichert.
func (s *namensSpeicher) name(pid uint32) string {
	s.mu.Lock()
	if e, ok := s.m[pid]; ok && time.Now().Before(e.bis) {
		s.mu.Unlock()
		return e.name
	}
	s.mu.Unlock()
	n := ""
	if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid); err == nil {
		puffer := make([]uint16, windows.MAX_LONG_PATH)
		l := uint32(len(puffer))
		if windows.QueryFullProcessImageName(h, 0, &puffer[0], &l) == nil {
			n = strings.ToLower(filepath.Base(windows.UTF16ToString(puffer[:l])))
		}
		windows.CloseHandle(h)
	}
	s.mu.Lock()
	if len(s.m) > 4096 {
		s.m = map[uint32]namenEintrag{}
	}
	s.m[pid] = namenEintrag{name: n, bis: time.Now().Add(20 * time.Second)}
	s.mu.Unlock()
	return n
}
