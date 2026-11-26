package main

import (
	"context"
	"log"

	"github.com/Dschonas04/bondr/internal/android"
	"github.com/Dschonas04/bondr/internal/tunnel"
	"github.com/Dschonas04/bondr/internal/verteiler"
)

// tunnelLaufen holt den VPN-Tunnel von der App und verteilt dessen Verkehr. Ohne
// Tunnel hat Bondr unter Android keinen Sinn, dann endet der Kern.
func tunnelLaufen(ctx context.Context, v *verteiler.Verteiler, stopp func()) {
	fd, err := android.Tunnel()
	if err != nil {
		log.Printf("Tunnel: %v", err)
		stopp()
		return
	}
	if err := tunnel.Laufen(ctx, fd, v); err != nil {
		log.Printf("Tunnel: %v", err)
	}
	stopp()
}
