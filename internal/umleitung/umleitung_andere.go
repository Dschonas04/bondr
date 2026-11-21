//go:build !windows

package umleitung

import "errors"

// Laufen gibt es nur unter Windows.
func Laufen(proxyAdresse, apiAdresse string) error {
	return errors.New("die Umleitung gibt es nur unter Windows")
}

// Einrichten gibt es nur unter Windows.
func Einrichten() error { return errors.New("die Umleitung gibt es nur unter Windows") }
