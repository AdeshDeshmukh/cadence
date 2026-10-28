package audio

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	// ErrDeviceNotFound is returned when a requested device is not found.
	ErrDeviceNotFound = errors.New("audio: device not found")

	// ErrNoInputDevices is returned when no audio input devices are detected.
	ErrNoInputDevices = errors.New("audio: no input devices available")

	// ErrDeviceNotInput is returned when an operation expects an input device but received an output device.
	ErrDeviceNotInput = errors.New("audio: selected device is not an input device")
)

// DeviceEnumerator is the interface for discovering physical/virtual audio devices on the system.
type DeviceEnumerator interface {
	EnumerateDevices() ([]Device, error)
}

// Manager handles audio device discovery, active selection, and fallback resolution.
type Manager struct {
	mu          sync.RWMutex
	enumerator  DeviceEnumerator
	devices     []Device
	selectedID  string
	lastRefresh time.Time
}

// NewManager initializes a DeviceManager with the provided enumerator.
// If enumerator is nil, NewDefaultEnumerator() is used.
func NewManager(enumerator DeviceEnumerator) *Manager {
	if enumerator == nil {
		enumerator = NewDefaultEnumerator()
	}
	m := &Manager{
		enumerator: enumerator,
	}
	_ = m.Refresh()
	return m
}

// Refresh re-scans the system for connected audio devices.
func (m *Manager) Refresh() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	devices, err := m.enumerator.EnumerateDevices()
	if err != nil {
		return err
	}

	m.devices = devices
	m.lastRefresh = time.Now()
	return nil
}

// AllDevices returns a copy of all discovered audio devices (input and output).
func (m *Manager) AllDevices() []Device {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Device, len(m.devices))
	copy(out, m.devices)
	return out
}

// InputDevices returns a list of only input devices.
func (m *Manager) InputDevices() []Device {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var inputs []Device
	for _, d := range m.devices {
		if d.Input {
			inputs = append(inputs, d)
		}
	}
	return inputs
}

// DefaultInputDevice returns the default system input device.
// If no device is explicitly marked default, it returns the first available input device.
func (m *Manager) DefaultInputDevice() (Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var firstInput *Device
	for _, d := range m.devices {
		if d.Input {
			if d.Default {
				return d, nil
			}
			if firstInput == nil {
				devCopy := d
				firstInput = &devCopy
			}
		}
	}

	if firstInput != nil {
		return *firstInput, nil
	}

	return Device{}, ErrNoInputDevices
}

// FindDevice searches for an input or output device by ID or name (case-insensitive).
func (m *Manager) FindDevice(query string) (Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if query == "" {
		return Device{}, ErrDeviceNotFound
	}

	lowerQuery := strings.ToLower(query)

	// 1. Exact ID match
	for _, d := range m.devices {
		if d.ID == query {
			return d, nil
		}
	}

	// 2. Exact Name match (case-insensitive)
	for _, d := range m.devices {
		if strings.EqualFold(d.Name, query) {
			return d, nil
		}
	}

	// 3. Substring Name match
	for _, d := range m.devices {
		if strings.Contains(strings.ToLower(d.Name), lowerQuery) {
			return d, nil
		}
	}

	return Device{}, ErrDeviceNotFound
}

// Select sets the active input device by ID, Name, or partial match.
func (m *Manager) Select(query string) error {
	dev, err := m.FindDevice(query)
	if err != nil {
		return err
	}

	if !dev.Input {
		return ErrDeviceNotInput
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.selectedID = dev.ID
	return nil
}

// SelectedDevice returns the currently selected input device.
// If no device has been explicitly selected or if the selected device was disconnected,
// it automatically falls back to the default input device.
func (m *Manager) SelectedDevice() (Device, error) {
	m.mu.RLock()
	selectedID := m.selectedID
	m.mu.RUnlock()

	if selectedID != "" {
		dev, err := m.FindDevice(selectedID)
		if err == nil && dev.Input {
			return dev, nil
		}
	}

	// Fallback to default
	return m.DefaultInputDevice()
}

// IsSelectedAvailable checks if the explicitly chosen device is currently present.
func (m *Manager) IsSelectedAvailable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.selectedID == "" {
		return false
	}

	for _, d := range m.devices {
		if d.ID == m.selectedID && d.Input {
			return true
		}
	}
	return false
}

// ClearSelection resets the active device selection back to automatic default.
func (m *Manager) ClearSelection() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.selectedID = ""
}

// LastRefresh returns the timestamp of the last successful device enumeration.
func (m *Manager) LastRefresh() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastRefresh
}

// -------------------------------------------------------------------------
// Platform Enumerators
// -------------------------------------------------------------------------

// NewDefaultEnumerator creates the appropriate device enumerator for the current operating system.
func NewDefaultEnumerator() DeviceEnumerator {
	if runtime.GOOS == "darwin" {
		return &DarwinAudioEnumerator{}
	}
	return &FallbackEnumerator{}
}

// FallbackEnumerator provides a static fallback input device for non-macOS or headless environments.
type FallbackEnumerator struct{}

func (f *FallbackEnumerator) EnumerateDevices() ([]Device, error) {
	return []Device{
		{
			ID:         "default",
			Name:       "Default System Audio Input",
			SampleRate: DefaultSampleRate,
			Input:      true,
			Default:    true,
		},
	}, nil
}

// DarwinAudioEnumerator queries macOS audio hardware.
type DarwinAudioEnumerator struct{}

type spAudioOutput struct {
	SPAudioDataType []struct {
		Items []spAudioItem `json:"_items"`
	} `json:"SPAudioDataType"`
}

type spAudioItem struct {
	Name           string      `json:"_name"`
	InputChannels  int         `json:"coreaudio_device_input"`
	OutputChannels int         `json:"coreaudio_device_output"`
	SampleRate     interface{} `json:"coreaudio_device_srate"`
	DefaultInput   string      `json:"coreaudio_default_audio_input_device"`
	DefaultOutput  string      `json:"coreaudio_default_audio_output_device"`
	Manufacturer   string      `json:"coreaudio_device_manufacturer"`
	TransportType  string      `json:"coreaudio_device_transport"`
}

func (d *DarwinAudioEnumerator) EnumerateDevices() ([]Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "system_profiler", "SPAudioDataType", "-json")
	output, err := cmd.Output()
	if err != nil {
		// Fallback if system_profiler is unavailable
		return (&FallbackEnumerator{}).EnumerateDevices()
	}

	return parseSPAudioJSON(output)
}

func parseSPAudioJSON(data []byte) ([]Device, error) {
	var prof spAudioOutput
	if err := json.Unmarshal(data, &prof); err != nil {
		return (&FallbackEnumerator{}).EnumerateDevices()
	}

	var devices []Device
	for _, dataType := range prof.SPAudioDataType {
		for _, item := range dataType.Items {
			if item.Name == "" {
				continue
			}

			srate := 16000
			switch val := item.SampleRate.(type) {
			case float64:
				srate = int(val)
			case int:
				srate = val
			}

			isInput := item.InputChannels > 0
			isDefault := item.DefaultInput == "spaudio_yes"

			// Create a sanitized stable ID
			id := strings.ToLower(strings.ReplaceAll(item.Name, " ", "-"))

			devices = append(devices, Device{
				ID:         id,
				Name:       item.Name,
				SampleRate: srate,
				Input:      isInput,
				Default:    isDefault,
			})
		}
	}

	if len(devices) == 0 {
		return (&FallbackEnumerator{}).EnumerateDevices()
	}

	return devices, nil
}
