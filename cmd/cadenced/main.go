package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/AdeshDeshmukh/cadence/internal/config"
	"github.com/AdeshDeshmukh/cadence/internal/daemon"
	"github.com/AdeshDeshmukh/cadence/pkg/version"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	cfg := config.Default()
	if err := config.Validate(cfg); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	app := daemon.NewApp(cfg)

	log.Printf("cadenced %s starting (shortcut: %s, mode: %s)",
		version.Version, cfg.Hotkey.Shortcut, cfg.Hotkey.Mode)

	if err := app.Run(ctx); err != nil {
		log.Fatalf("daemon runtime error: %v", err)
	}

	log.Println("cadenced stopped cleanly")
}
