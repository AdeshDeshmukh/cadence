package audio

import (
	"bufio"
	"strconv"
	"strings"
)

// ParsePactlSources parses the output of `pactl list sources short` into a slice of Device structs.
func ParsePactlSources(output, defaultSource string) []Device {
	var devices []Device
	scanner := bufio.NewScanner(strings.NewReader(output))
	defaultSource = strings.TrimSpace(defaultSource)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Field 0: index (e.g., "1")
		// Field 1: source identifier name (e.g., "alsa_input.pci-0000_00_1f.3.analog-stereo")
		id := fields[1]

		// Filter whether it is an input microphone or an output monitor
		isInput := !strings.HasSuffix(id, ".monitor")

		// Extract sample rate if present (e.g., "44100Hz" in field 3 or 4)
		srate := DefaultSampleRate
		for _, f := range fields[2:] {
			if strings.HasSuffix(f, "Hz") || strings.HasSuffix(f, "hz") {
				raw := strings.TrimSuffix(strings.TrimSuffix(f, "Hz"), "hz")
				if val, err := strconv.Atoi(raw); err == nil && val > 0 {
					srate = val
					break
				}
			}
		}

		friendlyName := formatLinuxDeviceName(id)
		isDefault := (defaultSource != "" && id == defaultSource)

		devices = append(devices, Device{
			ID:         id,
			Name:       friendlyName,
			SampleRate: srate,
			Input:      isInput,
			Default:    isDefault,
		})
	}

	// If no device was explicitly marked default, mark the first input device as default
	if len(devices) > 0 {
		hasDefault := false
		var firstInputIdx = -1
		for i, d := range devices {
			if d.Default && d.Input {
				hasDefault = true
				break
			}
			if d.Input && firstInputIdx == -1 {
				firstInputIdx = i
			}
		}
		if !hasDefault && firstInputIdx >= 0 {
			devices[firstInputIdx].Default = true
		}
	}

	return devices
}

// formatLinuxDeviceName converts raw ALSA/Pulse/PipeWire source identifiers into clean readable names.
func formatLinuxDeviceName(raw string) string {
	name := raw
	// Strip common prefixes
	for _, p := range []string{"alsa_input.", "alsa_output.", "pipewire.", "pulseaudio."} {
		name = strings.TrimPrefix(name, p)
	}

	isMonitor := strings.HasSuffix(name, ".monitor")
	name = strings.TrimSuffix(name, ".monitor")

	// Replace underscores and dashes with spaces
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, ".", " ")

	// Capitalize words
	words := strings.Fields(name)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	formatted := strings.Join(words, " ")

	if isMonitor {
		return formatted + " (Monitor)"
	}
	return formatted
}
