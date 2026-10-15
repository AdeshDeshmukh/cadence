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
}
