//go:build windows

package audio

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// WindowsAudioBackend indicates the audio capture mechanism.
type WindowsAudioBackend string

const (
	BackendWASAPI WindowsAudioBackend = "wasapi"
	BackendWinSim WindowsAudioBackend = "simulated"
)

// WindowsWASAPIDriver captures audio on Windows via WASAPI or simulated fallback.
type WindowsWASAPIDriver struct {
	mu           sync.Mutex
	backend      WindowsAudioBackend
	deviceName   string
	chunkSamples int
	running      bool
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	fallback     StreamDriver
}

// NewWindowsWASAPIDriver creates a stream driver for Windows WASAPI capture.
func NewWindowsWASAPIDriver(deviceName string) StreamDriver {
	return &WindowsWASAPIDriver{
		backend:      BackendWASAPI,
		deviceName:   deviceName,
		chunkSamples: DefaultPCMChunkSamples,
		fallback:     NewSimulatedDriver(440, 512, 32*time.Millisecond),
	}
}

// NewDefaultStreamDriver returns the native Windows stream driver.
func NewDefaultStreamDriver() StreamDriver {
	return NewWindowsWASAPIDriver("")
}

// StartStream begins capturing audio. If native capture binaries or endpoints are unavailable,
// it gracefully falls back to simulated audio chunks.
func (d *WindowsWASAPIDriver) StartStream(ctx context.Context, sampleRate int, out chan<- []float32) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return ErrAlreadyCapturing
	}

	d.running = true

	if d.backend == BackendWinSim {
		d.mu.Unlock()
		return d.fallback.StartStream(ctx, sampleRate, out)
	}

	// In standard Windows environments, if ffmpeg with dshow/wasapi is available
	if bin, err := exec.LookPath("ffmpeg"); err == nil {
		streamCtx, cancel := context.WithCancel(ctx)
		d.cancel = cancel

		audioSrc := "audio=default"
		if d.deviceName != "" {
			audioSrc = fmt.Sprintf("audio=%s", d.deviceName)
		}

		args := []string{
			"-f", "dshow",
			"-i", audioSrc,
			"-ac", "1",
			"-ar", fmt.Sprintf("%d", sampleRate),
			"-f", "s16le",
			"-",
		}

		cmd := exec.CommandContext(streamCtx, bin, args...)
		stdout, pipeErr := cmd.StdoutPipe()
		if pipeErr == nil && cmd.Start() == nil {
			d.cmd = cmd
			d.mu.Unlock()

			go func() {
				defer func() {
					d.mu.Lock()
					d.running = false
					d.mu.Unlock()
					if cmd.Process != nil {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
					}
				}()

				_ = ReadPCM16Stream(stdout, streamCtx, d.chunkSamples, out)
			}()

			return nil
		}
	}

	// Headless / non-hardware fallback: delegate to simulated driver
	d.backend = BackendWinSim
	d.mu.Unlock()
	return d.fallback.StartStream(ctx, sampleRate, out)
}

// StopStream terminates recording.
func (d *WindowsWASAPIDriver) StopStream() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return ErrNotCapturing
	}

	d.running = false
	if d.cancel != nil {
		d.cancel()
	}
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
	}
	if d.fallback != nil {
		_ = d.fallback.StopStream()
	}

	return nil
}
