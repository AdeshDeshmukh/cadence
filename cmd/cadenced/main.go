package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/AdeshDeshmukh/cadence/internal/config"
	"github.com/AdeshDeshmukh/cadence/internal/daemon"
	"github.com/AdeshDeshmukh/cadence/internal/supervisor"
	"github.com/AdeshDeshmukh/cadence/pkg/version"
)

func main() {
	supervised := flag.Bool("supervised", true, "run under process supervisor watchdog with auto-restart")
	flag.Parse()

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

	log.Printf("cadenced %s starting (shortcut: %s, mode: %s, supervised: %t)",
		version.Version, cfg.Hotkey.Shortcut, cfg.Hotkey.Mode, *supervised)

	if !*supervised {
		app := daemon.NewApp(cfg)
		if err := app.Run(ctx); err != nil {
			log.Fatalf("daemon runtime error: %v", err)
		}
		log.Println("cadenced stopped cleanly")
		return
	}

	sup := supervisor.New(supervisor.DefaultConfig())
	runErr := sup.Run(ctx, func(c context.Context) error {
		app := daemon.NewApp(cfg)
		return app.Run(c)
	})

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		log.Fatalf("cadenced fatal supervisor error: %v", runErr)
	}

	log.Println("cadenced stopped cleanly")
}
