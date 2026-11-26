//go:build !windows && !darwin && (!linux || android)

package system

// Auf anderen Systemen gibt es keinen Systemproxy; die Funktionen tun nichts.

func ProxySetzen(string) error    { return nil }
func ProxyEntfernen(string) error { return nil }
func ProxyAktiv(string) bool      { return false }
