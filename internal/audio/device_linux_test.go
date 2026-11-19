//go:build linux

package audio

import (
	"context"
	"errors"
	"testing"
)

func TestLinuxAudioEnumerator_MockRunner(t *testing.T) {
	mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "pactl" && len(args) >= 3 && args[0] == "list" && args[1] == "sources" {
			return []byte(samplePactlOutput), nil
		}
		if name == "pactl" && len(args) >= 1 && args[0] == "get-default-source" {
			return []byte("alsa_input.usb-Blue_Microphones_Yeti_Nano-00.analog-stereo\n"), nil
		}
		return nil, errors.New("command not found")
	}

	enum := NewLinuxAudioEnumeratorWithRunner(mockRunner)
	devices, err := enum.EnumerateDevices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}

	var hasDefault bool
	for _, d := range devices {
		if d.Default {
			hasDefault = true
			if d.ID != "alsa_input.usb-Blue_Microphones_Yeti_Nano-00.analog-stereo" {
				t.Errorf("unexpected default device ID: %s", d.ID)
			}
		}
	}
	if !hasDefault {
		t.Errorf("expected default device to be marked")
	}
}

func TestLinuxAudioEnumerator_FallbackOnFailure(t *testing.T) {
	failingRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("pactl failed")
	}

	enum := NewLinuxAudioEnumeratorWithRunner(failingRunner)
	devices, err := enum.EnumerateDevices()
	if err != nil {
		t.Fatalf("unexpected error on fallback: %v", err)
	}

	if len(devices) == 0 {
		t.Fatalf("expected fallback device when runner fails")
	}
	if devices[0].ID != "default" {
		t.Errorf("expected fallback device ID 'default', got %q", devices[0].ID)
	}
}
