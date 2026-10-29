package config

import "fmt"

func Validate(c Config) error {
	if c.Audio.SampleRate <= 0 {
		return fmt.Errorf("audio.sample_rate must be positive")
	}
	if c.Audio.Channels <= 0 {
		return fmt.Errorf("audio.channels must be positive")
	}

	if c.Hotkey.Shortcut == "" {
		return fmt.Errorf("hotkey.shortcut must not be empty")
	}
	if c.Hotkey.Mode != "" && c.Hotkey.Mode != "push_to_talk" && c.Hotkey.Mode != "toggle" {
		return fmt.Errorf("invalid hotkey.mode")
	}

	switch c.Privacy.NetworkMode {
	case "local_only", "hybrid", "cloud":
		return nil
	default:
		return fmt.Errorf("invalid privacy.network_mode")
	}
}
