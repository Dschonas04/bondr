package umleitung

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/Dschonas04/bondr/internal/einstellungen"
)

// Nur Ziele im Internet: LAN, Loopback, Link-Local und CGNAT gehen nie über Bondr.
// Nur Ziele im Internet: LAN, Loopback, Link-Local und CGNAT gehen nie über Bondr. WinDivert
// kennt "not" nur vor einzelnen Tests, deshalb jeder Bereich als "kleiner oder größer".
var filterOeffentlich = func() string {
	bereiche := [][2]string{
		{"10.0.0.0", "10.255.255.255"},
		{"100.64.0.0", "100.127.255.255"},
		{"127.0.0.0", "127.255.255.255"},
		{"169.254.0.0", "169.254.255.255"},
		{"172.16.0.0", "172.31.255.255"},
		{"192.168.0.0", "192.168.255.255"},
	}
	var teile []string
	for _, b := range bereiche {
		teile = append(teile, fmt.Sprintf("(ip.DstAddr < %s or ip.DstAddr > %s)", b[0], b[1]))
	}
	return strings.Join(append(teile, "ip.DstAddr < 224.0.0.0"), " and ")
}()

const firewallRegel = "Bondr Umleitung"

// Laufen fängt die Verbindungen der eingestellten Programme ab, bis die Umleitung
// ausgeschaltet wird oder bondrd nicht mehr läuft. Braucht Adminrechte.
func Laufen(proxyAdresse, apiAdresse string) error {
	m, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(`Local\Bondr-Umleitung`))
	if err != nil || windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		return errors.New("Umleitung läuft schon")
	}
	defer windows.CloseHandle(m)

	fehler := func(err error) error {
		statusSchreiben(Status{Fehler: err.Error()})
		return err
	}
	l, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", EmpfaengerPort))
	if err != nil {
		return fehler(fmt.Errorf("Port %d belegt: %w", EmpfaengerPort, err))
	}
	defer l.Close()
	firewallFreigeben()
	defer firewallEntfernen()

	filter := fmt.Sprintf("outbound and !loopback and ip and tcp and (tcp.SrcPort == %d or (%s))", EmpfaengerPort, filterOeffentlich)
	d, err := divertOeffnen(filter, 0)
	if err != nil {
		return fehler(err)
	}
	t := NeueTabelle()
	var aktiv atomic.Bool
	var programme atomic.Value
	programme.Store(map[string]bool{})
	namen := &namensSpeicher{m: map[uint32]namenEintrag{}}

	entscheiden := func(lokalPort uint16, ziel [4]byte, zielPort uint16) bool {
		if !aktiv.Load() {
			return false
		}
		pid := besitzer(lokalPort, ziel, zielPort)
		if pid == 0 || pid == uint32(os.Getpid()) {
			return false
		}
		return programme.Load().(map[string]bool)[namen.name(pid)]
	}

	ctx, stopp := context.WithCancel(context.Background())
	defer stopp()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pakete(ctx, d, t, entscheiden)
		}()
	}
	go annehmen(ctx, l, t, proxyAdresse)

	log.Printf("Umleitung läuft, Empfänger %d", EmpfaengerPort)
	start := time.Now()
	weg := 0
	for ctx.Err() == nil {
		w := einstellungen.Laden().Werte()
		p := map[string]bool{}
		for _, n := range w.Programme {
			p[strings.ToLower(n)] = true
		}
		programme.Store(p)
		if !w.Umleitung {
			log.Printf("Umleitung ausgeschaltet")
			break
		}
		apiDa := erreichbar(apiAdresse)
		if apiDa {
			weg = 0
		} else if time.Since(start) > 20*time.Second {
			weg++
		}
		if weg >= 3 {
			log.Printf("bondrd läuft nicht mehr, Umleitung endet")
			break
		}
		aktiv.Store(apiDa && erreichbar(proxyAdresse))
		t.Aufraeumen(time.Now().UnixNano(), int64(10*time.Minute))
		statusSchreiben(Status{Aktiv: aktiv.Load(), Umgeleitet: t.Umgeleitet(), Offen: t.Offen()})
		time.Sleep(time.Second)
	}
	stopp()
	d.beenden()
	wg.Wait()
	statusSchreiben(Status{Umgeleitet: t.Umgeleitet()})
	os.Remove(StatusDatei())
	return nil
}

func pakete(ctx context.Context, d *divert, t *Tabelle, entscheiden func(uint16, [4]byte, uint16) bool) {
	puffer := make([]byte, mtuMax)
	var a adresse
	for ctx.Err() == nil {
		n, err := d.empfangen(puffer, &a)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
				continue
			}
			log.Printf("WinDivert empfangen: %v", err)
			time.Sleep(10 * time.Millisecond)
			continue
		}
		p := puffer[:n]
		switch t.Bearbeiten(p, EmpfaengerPort, time.Now().UnixNano(), entscheiden) {
		case Verwerfen:
			continue
		case Spiegeln:
			a.Bits &^= bitAusgehend
			d.pruefsummen(p, &a)
		}
		if err := d.senden(p, &a); err != nil && ctx.Err() == nil {
			log.Printf("WinDivert senden: %v", err)
		}
	}
}

// annehmen nimmt gespiegelte Verbindungen an und reicht sie über den SOCKS-Proxy von bondrd
// an ihr ursprüngliches Ziel weiter.
func annehmen(ctx context.Context, l net.Listener, t *Tabelle, proxyAdresse string) {
	go func() { <-ctx.Done(); l.Close() }()
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		go func() {
			defer c.Close()
			ziel, ok := t.Ziel(c.RemoteAddr().(*net.TCPAddr))
			if !ok {
				return
			}
			z, err := socks(proxyAdresse, ziel)
			if err != nil {
				log.Printf("Umleitung %s: %v", ziel, err)
				return
			}
			defer z.Close()
			durchreichen(c, z)
		}()
	}
}

func socks(proxyAdresse string, ziel *net.TCPAddr) (net.Conn, error) {
	z, err := net.DialTimeout("tcp", proxyAdresse, 5*time.Second)
	if err != nil {
		return nil, err
	}
	z.SetDeadline(time.Now().Add(30 * time.Second))
	anfrage := []byte{5, 1, 0, 5, 1, 0, 1}
	anfrage = append(anfrage, ziel.IP.To4()...)
	anfrage = binary.BigEndian.AppendUint16(anfrage, uint16(ziel.Port))
	if _, err := z.Write(anfrage); err != nil {
		z.Close()
		return nil, err
	}
	antwort := make([]byte, 12) // 2 Byte Methode, 10 Byte Antwort mit IPv4
	if _, err := io.ReadFull(z, antwort); err != nil {
		z.Close()
		return nil, err
	}
	if antwort[0] != 5 || antwort[1] != 0 || antwort[3] != 0 {
		z.Close()
		return nil, fmt.Errorf("Proxy lehnt ab (%d)", antwort[3])
	}
	z.SetDeadline(time.Time{})
	return z, nil
}

func durchreichen(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	kopieren := func(nach, von net.Conn) {
		defer wg.Done()
		io.Copy(nach, von)
		if cw, ok := nach.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		} else {
			nach.Close()
		}
	}
	go kopieren(a, b)
	go kopieren(b, a)
	wg.Wait()
}

func erreichbar(adresse string) bool {
	c, err := net.DialTimeout("tcp", adresse, 500*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func versteckt(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return c
}

// Die gespiegelten Verbindungen kommen für die Windows-Firewall von außen, deshalb braucht
// der Empfänger eine Freigabe, aber nur für bondrd.exe und nur auf seinem Port.
func firewallFreigeben() {
	firewallEntfernen()
	exe, _ := os.Executable()
	versteckt("netsh", "advfirewall", "firewall", "add", "rule", "name="+firewallRegel, "dir=in", "action=allow",
		"protocol=TCP", "localport="+strconv.Itoa(EmpfaengerPort), "program="+exe, "enable=yes").Run()
}

func firewallEntfernen() {
	versteckt("netsh", "advfirewall", "firewall", "delete", "rule", "name="+firewallRegel).Run()
}

// Einrichten legt mit Adminrechten die geplante Aufgabe an, über die die App den Umleiter
// später ohne Rückfrage startet, und startet ihn.
func Einrichten() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	xml := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>Bondr: Verbindungen von Steam und anderen Programmen über alle Wege verteilen</Description></RegistrationInfo>
  <Principals>
    <Principal id="Benutzer">
      <UserId>` + u.User.Sid.String() + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
    <Priority>4</Priority>
  </Settings>
  <Actions Context="Benutzer">
    <Exec><Command>` + xmlText(exe) + `</Command><Arguments>--umleitung</Arguments></Exec>
  </Actions>
</Task>`
	datei := filepath.Join(os.TempDir(), "bondr-aufgabe.xml")
	inhalt := []byte{0xFF, 0xFE}
	for _, z := range utf16.Encode([]rune(xml)) {
		inhalt = append(inhalt, byte(z), byte(z>>8))
	}
	if err := os.WriteFile(datei, inhalt, 0o600); err != nil {
		return err
	}
	defer os.Remove(datei)
	if aus, err := versteckt("schtasks", "/create", "/tn", Aufgabe, "/xml", datei, "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("Aufgabe anlegen: %v %s", err, strings.TrimSpace(string(aus)))
	}
	if aus, err := versteckt("schtasks", "/run", "/tn", Aufgabe).CombinedOutput(); err != nil {
		return fmt.Errorf("Aufgabe starten: %v %s", err, strings.TrimSpace(string(aus)))
	}
	return nil
}

func xmlText(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
