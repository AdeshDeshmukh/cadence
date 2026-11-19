//go:build linux

package audio

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// LinuxAudioBackend represents the sound server used for recording.
type LinuxAudioBackend string

const (
	BackendPipeWire   LinuxAudioBackend = "pipewire"
	BackendPulseAudio LinuxAudioBackend = "pulseaudio"
	BackendSimulated  LinuxAudioBackend = "simulated"
)

// DetectLinuxAudioBackend determines whether PipeWire, PulseAudio, or simulated fallback should be used.
func DetectLinuxAudioBackend() LinuxAudioBackend {
	if _, err := exec.LookPath("pw-record"); err == nil {
		return BackendPipeWire
	}
	if _, err := exec.LookPath("parec"); err == nil {
		return BackendPulseAudio
	}
	if _, err := exec.LookPath("pacat"); err == nil {
		return BackendPulseAudio
	}
	return BackendSimulated
}

// LinuxAudioDriver captures audio on Linux via PipeWire (pw-record) or PulseAudio (parec).
type LinuxAudioDriver struct {
	mu           sync.Mutex
	backend      LinuxAudioBackend
	deviceName   string
	chunkSamples int
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	running      bool
	fallback     StreamDriver
}

// NewLinuxAudioDriver creates a stream driver for Linux.
func NewLinuxAudioDriver(deviceName string) StreamDriver {
	backend := DetectLinuxAudioBackend()

	return &LinuxAudioDriver{
		backend:      backend,
		deviceName:   deviceName,
		chunkSamples: DefaultPCMChunkSamples,
		fallback:     NewSimulatedDriver(440, 512, 32*time.Millisecond),
	}
}

// NewDefaultStreamDriver returns the native Linux stream driver.
func NewDefaultStreamDriver() StreamDriver {
	return NewLinuxAudioDriver("")
}

// StartStream begins capturing audio and emitting float32 sample chunks to out.
func (d *LinuxAudioDriver) StartStream(ctx context.Context, sampleRate int, out chan<- []float32) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return ErrAlreadyCapturing
	}

	if d.backend == BackendSimulated {
		d.running = true
		d.mu.Unlock()
		return d.fallback.StartStream(ctx, sampleRate, out)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	var bin string
	var args []string

	switch d.backend {
	case BackendPipeWire:
		bin = "pw-record"
		args = []string{"--channels=1", fmt.Sprintf("--rate=%d", sampleRate), "--format=s16", "-"}
		if d.deviceName != "" {
			args = append(args, fmt.Sprintf("--target=%s", d.deviceName))
		}
	case BackendPulseAudio:
		bin = "parec"
		args = []string{"--channels=1", fmt.Sprintf("--rate=%d", sampleRate), "--format=s16le", "--raw"}
		if d.deviceName != "" {
			args = append(args, fmt.Sprintf("--device=%s", d.deviceName))
		}
	default:
		d.backend = BackendSimulated
		d.running = true
		d.mu.Unlock()
		return d.fallback.StartStream(ctx, sampleRate, out)
	}

	cmd := exec.CommandContext(streamCtx, bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		d.backend = BackendSimulated
		d.running = true
		d.mu.Unlock()
		// Fallback to simulated if pipe creation fails
		return d.fallback.StartStream(ctx, sampleRate, out)
	}

	if err := cmd.Start(); err != nil {
		d.backend = BackendSimulated
		d.running = true
		d.mu.Unlock()
		// Fallback to simulated if command start fails (e.g., daemon not running)
		return d.fallback.StartStream(ctx, sampleRate, out)
	}

	d.cmd = cmd
	d.running = true
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

// StopStream terminates the active recording process.
func (d *LinuxAudioDriver) StopStream() error {
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
	if d.backend == BackendSimulated && d.fallback != nil {
		_ = d.fallback.StopStream()
	}

	return nil
}
