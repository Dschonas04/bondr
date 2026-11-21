// Paket system stellt den Windows-Proxy und den Autostart ein.
package system

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	internetEinstellungen         = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	ausnahmen                     = "<local>;localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.2*;172.30.*;172.31.*;192.168.*;*.local;*.fritz.box"
	internetOptionSettingsChanged = 39
	internetOptionRefresh         = 37
)

var wininet = windows.NewLazySystemDLL("wininet.dll")

// ProxySetzen trägt Bondr als Proxy für alle Programme ein, die den Windows-Proxy
// beachten (Browser, Store, die meisten Launcher).
func ProxySetzen(adresse string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, internetEinstellungen, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("ProxyServer", adresse); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyOverride", ausnahmen); err != nil {
		return err
	}
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	benachrichtigen()
	return nil
}

// ProxyEntfernen schaltet den Windows-Proxy wieder ab, aber nur, wenn er noch auf
// Bondr zeigt. Einen fremden Proxy lässt es stehen.
func ProxyEntfernen(adresse string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetEinstellungen, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if v, _, _ := k.GetStringValue("ProxyServer"); v != adresse {
		return nil
	}
	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return err
	}
	benachrichtigen()
	return nil
}

// ProxyAktiv sagt, ob der Windows-Proxy gerade auf adresse zeigt.
func ProxyAktiv(adresse string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetEinstellungen, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	an, _, _ := k.GetIntegerValue("ProxyEnable")
	v, _, _ := k.GetStringValue("ProxyServer")
	return an == 1 && v == adresse
}

func benachrichtigen() {
	f := wininet.NewProc("InternetSetOptionW")
	if f.Find() != nil {
		return
	}
	f.Call(0, internetOptionSettingsChanged, 0, 0)
	f.Call(0, internetOptionRefresh, 0, 0)
}
