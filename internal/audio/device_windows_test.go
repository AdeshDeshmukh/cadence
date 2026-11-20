//go:build windows

package audio

import (
	"context"
	"errors"
	"testing"
)

func TestParseWindowsAudioJSON_Single(t *testing.T) {
	sampleJSON := []byte(`{
		"DeviceId": "SWD\\MMDEVAPI\\{0.0.1.00000000}.{GUID}",
		"Name": "Realtek High Definition Audio",
		"Status": "OK"
	}`)

	devices := ParseWindowsAudioJSON(sampleJSON)
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	dev := devices[0]
	if dev.Name != "Realtek High Definition Audio" {
		t.Errorf("unexpected name: %s", dev.Name)
	}
	if !dev.Input || !dev.Default {
		t.Errorf("expected input and default to be true")
	}
	if dev.SampleRate != DefaultSampleRate {
		t.Errorf("expected sample rate %d, got %d", DefaultSampleRate, dev.SampleRate)
	}
}

func TestParseWindowsAudioJSON_Array(t *testing.T) {
	sampleJSON := []byte(`[
		{
			"DeviceId": "DEV_MIC_1",
			"Name": "USB Microphone",
			"Status": "OK"
		},
		{
			"DeviceId": "DEV_HEADSET_2",
			"Name": "Wireless Headset",
			"Status": "OK"
		}
	]`)

	devices := ParseWindowsAudioJSON(sampleJSON)
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	if devices[0].Name != "USB Microphone" || !devices[0].Default {
		t.Errorf("expected first device to be USB Microphone and default")
	}
	if devices[1].Name != "Wireless Headset" || devices[1].Default {
		t.Errorf("expected second device to not be default")
	}
}

func TestWindowsAudioEnumerator_Mock(t *testing.T) {
	mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`[{"DeviceId": "DEV1", "Name": "Microphone Array", "Status": "OK"}]`), nil
	}

	enum := NewWindowsAudioEnumeratorWithRunner(mockRunner)
	devices, err := enum.EnumerateDevices()
	if err != nil {
		t.Fatalf("EnumerateDevices failed: %v", err)
	}

	if len(devices) != 1 || devices[0].Name != "Microphone Array" {
		t.Fatalf("unexpected devices: %+v", devices)
	}
}

func TestWindowsAudioEnumerator_Fallback(t *testing.T) {
	mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("powershell execution failed")
	}

	enum := NewWindowsAudioEnumeratorWithRunner(mockRunner)
	devices, err := enum.EnumerateDevices()
	if err != nil {
		t.Fatalf("EnumerateDevices fallback failed: %v", err)
	}

	if len(devices) != 1 || devices[0].ID != "default" {
		t.Fatalf("expected fallback device, got %+v", devices)
	}
}
