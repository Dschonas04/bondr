//go:build !windows

package system

import (
	"os/exec"
	"path/filepath"
)

// Außerhalb von Windows gibt es keinen Systemproxy; die Funktionen tun nichts.

func ProxySetzen(string) error    { return nil }
func ProxyEntfernen(string) error { return nil }
func ProxyAktiv(string) bool      { return false }
func Autostart(bool) error        { return nil }

func Oeffnen(url string) { exec.Command("xdg-open", url).Start() }

func OrdnerZeigen(datei string) { exec.Command("xdg-open", filepath.Dir(datei)).Start() }
