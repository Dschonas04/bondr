//go:build !android

package main

import (
	"context"
	"log"

	"github.com/Dschonas04/bondr/internal/verteiler"
)

func tunnelLaufen(_ context.Context, _ *verteiler.Verteiler, _ func()) {
	log.Printf("Tunnel gibt es nur unter Android")
}
