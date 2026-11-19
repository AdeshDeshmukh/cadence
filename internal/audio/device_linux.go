//go:build linux

package audio

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// LinuxAudioEnumerator discovers input devices using pactl or pw-cli.
type LinuxAudioEnumerator struct {
	runner CommandRunner
}

// CommandRunner allows executing shell commands (mockable for testing).
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func defaultCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// NewLinuxAudioEnumerator creates a device enumerator for Linux audio systems.
func NewLinuxAudioEnumerator() DeviceEnumerator {
	return &LinuxAudioEnumerator{runner: defaultCommandRunner}
}

// NewLinuxAudioEnumeratorWithRunner creates an enumerator with a custom command runner.
func NewLinuxAudioEnumeratorWithRunner(runner CommandRunner) DeviceEnumerator {
	if runner == nil {
		runner = defaultCommandRunner
	}
	return &LinuxAudioEnumerator{runner: runner}
}

func (l *LinuxAudioEnumerator) EnumerateDevices() ([]Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sourcesOut, err := l.runner(ctx, "pactl", "list", "sources", "short")
	if err == nil && len(sourcesOut) > 0 {
		defaultSource := ""
		if defOut, defErr := l.runner(ctx, "pactl", "get-default-source"); defErr == nil {
			defaultSource = strings.TrimSpace(string(defOut))
		}
		devices := ParsePactlSources(string(sourcesOut), defaultSource)
		if len(devices) > 0 {
			return devices, nil
		}
	}

	return (&FallbackEnumerator{}).EnumerateDevices()
}

func newPlatformEnumerator() DeviceEnumerator {
	return NewLinuxAudioEnumerator()
}
