package audio

import (
	"testing"
)

const samplePactlOutput = `
0	alsa_output.pci-0000_00_1f.3.analog-stereo.monitor	module-alsa-card.c	s16le 2ch 44100Hz	SUSPENDED
1	alsa_input.pci-0000_00_1f.3.analog-stereo	module-alsa-card.c	s16le 2ch 44100Hz	SUSPENDED
2	alsa_input.usb-Blue_Microphones_Yeti_Nano-00.analog-stereo	module-alsa-card.c	s16le 2ch 48000Hz	RUNNING
`

func TestParsePactlSources(t *testing.T) {
	devices := ParsePactlSources(samplePactlOutput, "alsa_input.usb-Blue_Microphones_Yeti_Nano-00.analog-stereo")
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}

	// 1. Output monitor device
	mon := devices[0]
	if mon.Input {
		t.Errorf("expected monitor device to have Input=false")
	}
	if mon.SampleRate != 44100 {
		t.Errorf("expected 44100Hz, got %d", mon.SampleRate)
	}

	// 2. Built-in analog input
	builtin := devices[1]
	if !builtin.Input {
		t.Errorf("expected built-in microphone to have Input=true")
	}
	if builtin.Default {
		t.Errorf("expected built-in not to be default")
	}

	// 3. USB Yeti Nano input (explicitly default)
	yeti := devices[2]
	if !yeti.Input {
		t.Errorf("expected Yeti mic to have Input=true")
	}
	if !yeti.Default {
		t.Errorf("expected Yeti mic to be default")
	}
	if yeti.SampleRate != 48000 {
		t.Errorf("expected 48000Hz, got %d", yeti.SampleRate)
	}
}

func TestParsePactlSources_FallbackDefault(t *testing.T) {
	// When defaultSource does not match, first input should be defaulted
	devices := ParsePactlSources(samplePactlOutput, "non_existent_source")
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}

	if devices[0].Default {
		t.Errorf("monitor should never be chosen as default input")
	}
	if !devices[1].Default {
		t.Errorf("first non-monitor input should have been selected as fallback default")
	}
}

func TestFormatLinuxDeviceName(t *testing.T) {
	raw := "alsa_input.usb-Blue_Microphones_Yeti_Nano-00.analog-stereo"
	formatted := formatLinuxDeviceName(raw)
	if formatted != "Usb Blue Microphones Yeti Nano 00 Analog Stereo" {
		t.Errorf("unexpected formatted name: %q", formatted)
	}

	monRaw := "alsa_output.pci-0000_00_1f.3.analog-stereo.monitor"
	monFormatted := formatLinuxDeviceName(monRaw)
	if monFormatted != "Pci 0000 00 1f 3 Analog Stereo (Monitor)" {
		t.Errorf("unexpected monitor formatted name: %q", monFormatted)
	}
}
