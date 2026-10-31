package config_test

import (
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.Default()
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("default config failed validation: %v", err)
	}
	if !cfg.Paste.PreserveClipboard {
		t.Fatal("expected default Paste.PreserveClipboard to be true")
	}
	if cfg.Paste.RestoreDelayMs != 80 {
		t.Fatalf("expected default Paste.RestoreDelayMs to be 80, got %d", cfg.Paste.RestoreDelayMs)
	}
}

func TestValidateConfigErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Audio.SampleRate = 0
	if err := config.Validate(cfg); err == nil {
		t.Fatal("expected error for non-positive sample rate")
	}

	cfg = config.Default()
	cfg.Audio.Channels = -1
	if err := config.Validate(cfg); err == nil {
		t.Fatal("expected error for non-positive channels")
	}

	cfg = config.Default()
	cfg.Privacy.NetworkMode = "invalid_mode"
	if err := config.Validate(cfg); err == nil {
		t.Fatal("expected error for invalid privacy network mode")
	}

	cfg = config.Default()
	cfg.Storage.DBPath = ""
	if err := config.Validate(cfg); err == nil {
		t.Fatal("expected error for empty storage db_path")
	}

	cfg = config.Default()
	cfg.Paste.RestoreDelayMs = -1
	if err := config.Validate(cfg); err == nil {
		t.Fatal("expected error for negative paste restore delay")
	}

	cfg = config.Default()
	cfg.VAD.Engine = "invalid_vad"
	if err := config.Validate(cfg); err == nil {
		t.Fatal("expected error for invalid vad engine")
	}

	if path := config.DefaultDBPath(); path == "" {
		t.Fatal("expected non-empty default storage db path")
	}
}
