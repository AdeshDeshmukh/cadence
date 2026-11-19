//go:build windows

package audio

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// WindowsAudioEnumerator discovers audio devices on Windows systems.
type WindowsAudioEnumerator struct {
	runner CommandRunner
}

// NewWindowsAudioEnumerator creates an enumerator for Windows.
func NewWindowsAudioEnumerator() DeviceEnumerator {
	return &WindowsAudioEnumerator{runner: defaultCommandRunner}
}

// NewWindowsAudioEnumeratorWithRunner creates an enumerator with a custom command runner.
func NewWindowsAudioEnumeratorWithRunner(runner CommandRunner) DeviceEnumerator {
	if runner == nil {
		runner = defaultCommandRunner
	}
	return &WindowsAudioEnumerator{runner: runner}
}

type winSoundDevice struct {
	DeviceID string `json:"DeviceId"`
	Name     string `json:"Name"`
	Status   string `json:"Status"`
}

func (w *WindowsAudioEnumerator) EnumerateDevices() ([]Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// PowerShell query for sound devices formatted as JSON
	psScript := `Get-CimInstance Win32_SoundDevice | Select-Object DeviceId, Name, Status | ConvertTo-Json`
	out, err := w.runner(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	if err == nil && len(out) > 0 {
		devices := ParseWindowsAudioJSON(out)
		if len(devices) > 0 {
			return devices, nil
		}
	}

	return (&FallbackEnumerator{}).EnumerateDevices()
}

// ParseWindowsAudioJSON parses JSON output from PowerShell Win32_SoundDevice query.
func ParseWindowsAudioJSON(data []byte) []Device {
	var single winSoundDevice
	if err := json.Unmarshal(data, &single); err == nil && single.Name != "" {
		return []Device{
			{
				ID:         sanitizeDeviceID(single.DeviceID, single.Name),
				Name:       single.Name,
				SampleRate: DefaultSampleRate,
				Input:      true,
				Default:    true,
			},
		}
	}

	var list []winSoundDevice
	if err := json.Unmarshal(data, &list); err == nil && len(list) > 0 {
		var devices []Device
		for i, dev := range list {
			if dev.Name == "" {
				continue
			}
			devices = append(devices, Device{
				ID:         sanitizeDeviceID(dev.DeviceID, dev.Name),
				Name:       dev.Name,
				SampleRate: DefaultSampleRate,
				Input:      true,
				Default:    i == 0,
			})
		}
		if len(devices) > 0 {
			return devices
		}
	}

	return nil
}

func sanitizeDeviceID(deviceID, name string) string {
	if deviceID != "" {
		return strings.ToLower(strings.ReplaceAll(deviceID, " ", "-"))
	}
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

func newPlatformEnumerator() DeviceEnumerator {
	return NewWindowsAudioEnumerator()
}
