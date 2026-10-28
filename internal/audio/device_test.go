package audio

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type mockEnumerator struct {
	devices []Device
	err     error
}

func (m *mockEnumerator) EnumerateDevices() ([]Device, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.devices, nil
}

func sampleDevices() []Device {
	return []Device{
		{ID: "mic-builtin", Name: "MacBook Air Microphone", SampleRate: 44100, Input: true, Default: true},
		{ID: "spk-builtin", Name: "MacBook Air Speakers", SampleRate: 44100, Input: false, Default: false},
		{ID: "airpods-pro", Name: "AirPods Pro", SampleRate: 48000, Input: true, Default: false},
	}
}

func TestDeviceManager_ListAndFilter(t *testing.T) {
	mock := &mockEnumerator{devices: sampleDevices()}
	mgr := NewManager(mock)

	all := mgr.AllDevices()
	if len(all) != 3 {
		t.Fatalf("expected 3 total devices, got %d", len(all))
	}

	inputs := mgr.InputDevices()
	if len(inputs) != 2 {
		t.Fatalf("expected 2 input devices, got %d", len(inputs))
	}
}

func TestDeviceManager_DefaultInput(t *testing.T) {
	// Case 1: Explicit default present
	mock := &mockEnumerator{devices: sampleDevices()}
	mgr := NewManager(mock)

	def, err := mgr.DefaultInputDevice()
	if err != nil {
		t.Fatalf("unexpected error getting default: %v", err)
	}
	if def.ID != "mic-builtin" {
		t.Fatalf("expected mic-builtin, got %s", def.ID)
	}

	// Case 2: No explicit default (falls back to first input)
	mockNoDef := &mockEnumerator{
		devices: []Device{
			{ID: "dev-1", Name: "Dev 1", Input: false},
			{ID: "dev-2", Name: "Dev 2", Input: true, Default: false},
			{ID: "dev-3", Name: "Dev 3", Input: true, Default: false},
		},
	}
	mgrNoDef := NewManager(mockNoDef)
	def2, err := mgrNoDef.DefaultInputDevice()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def2.ID != "dev-2" {
		t.Fatalf("expected dev-2, got %s", def2.ID)
	}

	// Case 3: No input devices at all
	mockEmpty := &mockEnumerator{
		devices: []Device{
			{ID: "spk", Name: "Speaker", Input: false},
		},
	}
	mgrEmpty := NewManager(mockEmpty)
	_, err = mgrEmpty.DefaultInputDevice()
	if !errors.Is(err, ErrNoInputDevices) {
		t.Fatalf("expected ErrNoInputDevices, got %v", err)
	}
}

func TestDeviceManager_FindDevice(t *testing.T) {
	mock := &mockEnumerator{devices: sampleDevices()}
	mgr := NewManager(mock)

	// By exact ID
	d1, err := mgr.FindDevice("mic-builtin")
	if err != nil || d1.ID != "mic-builtin" {
		t.Fatalf("failed to find by ID: %v", err)
	}

	// By exact Name
	d2, err := mgr.FindDevice("AirPods Pro")
	if err != nil || d2.ID != "airpods-pro" {
		t.Fatalf("failed to find by Name: %v", err)
	}

	// By substring (case-insensitive)
	d3, err := mgr.FindDevice("airpods")
	if err != nil || d3.ID != "airpods-pro" {
		t.Fatalf("failed to find by substring: %v", err)
	}

	// Non-existent
	_, err = mgr.FindDevice("unknown-mic")
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("expected ErrDeviceNotFound, got %v", err)
	}

	// Empty query
	_, err = mgr.FindDevice("")
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("expected ErrDeviceNotFound on empty query, got %v", err)
	}
}

func TestDeviceManager_SelectAndFallback(t *testing.T) {
	mock := &mockEnumerator{devices: sampleDevices()}
	mgr := NewManager(mock)

	// Initial selection is empty -> defaults to default input device
	sel, err := mgr.SelectedDevice()
	if err != nil || sel.ID != "mic-builtin" {
		t.Fatalf("expected default device, got %s (err: %v)", sel.ID, err)
	}
	if mgr.IsSelectedAvailable() {
		t.Fatalf("expected IsSelectedAvailable to be false when no explicit selection")
	}

	// Select AirPods
	if err := mgr.Select("airpods"); err != nil {
		t.Fatalf("failed to select airpods: %v", err)
	}
	if !mgr.IsSelectedAvailable() {
		t.Fatalf("expected IsSelectedAvailable true")
	}

	sel, err = mgr.SelectedDevice()
	if err != nil || sel.ID != "airpods-pro" {
		t.Fatalf("expected airpods-pro, got %s", sel.ID)
	}

	// Try selecting an output device (should fail)
	err = mgr.Select("Speakers")
	if !errors.Is(err, ErrDeviceNotInput) {
		t.Fatalf("expected ErrDeviceNotInput, got %v", err)
	}

	// Simulate device disconnection: AirPods unplugged!
	mock.devices = []Device{
		{ID: "mic-builtin", Name: "MacBook Air Microphone", Input: true, Default: true},
	}
	if err := mgr.Refresh(); err != nil {
		t.Fatalf("failed to refresh: %v", err)
	}

	if mgr.IsSelectedAvailable() {
		t.Fatalf("expected IsSelectedAvailable false after device unplugged")
	}

	// SelectedDevice should automatically fall back to mic-builtin
	fallback, err := mgr.SelectedDevice()
	if err != nil || fallback.ID != "mic-builtin" {
		t.Fatalf("expected fallback to mic-builtin, got %s", fallback.ID)
	}

	// Clear selection explicitly
	mgr.ClearSelection()
	if mgr.IsSelectedAvailable() {
		t.Fatalf("expected IsSelectedAvailable false after ClearSelection")
	}
}

func TestDeviceManager_RefreshError(t *testing.T) {
	mock := &mockEnumerator{err: errors.New("system audio service unavailable")}
	mgr := &Manager{enumerator: mock}
	err := mgr.Refresh()
	if err == nil {
		t.Fatalf("expected error on refresh failure")
	}
}

func TestDarwinAudioEnumerator_ParseJSON(t *testing.T) {
	rawJSON := []byte(`{
		"SPAudioDataType" : [
			{
				"_items" : [
					{
						"_name" : "USB Studio Microphone",
						"coreaudio_device_input" : 1,
						"coreaudio_default_audio_input_device" : "spaudio_yes",
						"coreaudio_device_srate" : 48000
					},
					{
						"_name" : "External Headphones",
						"coreaudio_device_output" : 2,
						"coreaudio_default_audio_output_device" : "spaudio_yes",
						"coreaudio_device_srate" : 44100
					}
				],
				"_name" : "coreaudio_device"
			}
		]
	}`)

	devs, err := parseSPAudioJSON(rawJSON)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(devs) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devs))
	}

	if devs[0].Name != "USB Studio Microphone" || !devs[0].Input || !devs[0].Default {
		t.Fatalf("unexpected mic properties: %+v", devs[0])
	}

	if devs[1].Name != "External Headphones" || devs[1].Input || devs[1].Default {
		t.Fatalf("unexpected headphone properties: %+v", devs[1])
	}
}

func TestDarwinAudioEnumerator_ParseBadJSON(t *testing.T) {
	devs, err := parseSPAudioJSON([]byte(`invalid-json`))
	if err != nil {
		t.Fatalf("expected fallback on bad JSON, got error: %v", err)
	}
	if len(devs) == 0 {
		t.Fatalf("expected fallback devices")
	}
}

func TestFallbackEnumerator(t *testing.T) {
	fb := &FallbackEnumerator{}
	devs, err := fb.EnumerateDevices()
	if err != nil || len(devs) != 1 {
		t.Fatalf("expected 1 fallback device, got %d (err: %v)", len(devs), err)
	}
	if !devs[0].Input || !devs[0].Default {
		t.Fatalf("expected fallback to be default input")
	}
}

func TestDeviceManager_ConcurrentRace(t *testing.T) {
	mock := &mockEnumerator{devices: sampleDevices()}
	mgr := NewManager(mock)
	var wg sync.WaitGroup

	numRoutines := 8
	iterations := 100

	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = mgr.AllDevices()
				_ = mgr.InputDevices()
				_, _ = mgr.DefaultInputDevice()
				_, _ = mgr.SelectedDevice()
				if j%10 == 0 {
					_ = mgr.Select("airpods")
				} else if j%10 == 5 {
					mgr.ClearSelection()
				}
				_ = mgr.LastRefresh()
				time.Sleep(10 * time.Microsecond)
			}
		}(i)
	}

	wg.Wait()
}
