package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/AdeshDeshmukh/cadence/pkg/version"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	log.Printf("cadenced %s started", version.Version)
	<-ctx.Done()
	log.Println("cadenced stopped")
}
