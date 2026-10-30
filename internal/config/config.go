package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Audio   AudioConfig
	ASR     ASRConfig
	Post    PostConfig
	Privacy PrivacyConfig
	Hotkey  HotkeyConfig
	IPC     IPCConfig
	Storage StorageConfig
}

type AudioConfig struct {
	SampleRate int
	Channels   int
}

type ASRConfig struct {
	Engine    string
	Model     string
	Language  string
	Streaming bool
}

type PostConfig struct {
	FillerRemoval bool
	Punctuation   bool
	Vocabulary    bool
	ITN           bool
}

type PrivacyConfig struct {
	NetworkMode string
	Telemetry   bool
}

type HotkeyConfig struct {
	Shortcut string
	Mode     string
}

type IPCConfig struct {
	SocketPath string
}

type StorageConfig struct {
	DBPath string
}

func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "cadence.db")
	}
	cadenceDir := filepath.Join(home, ".cadence")
	_ = os.MkdirAll(cadenceDir, 0755)
	return filepath.Join(cadenceDir, "cadence.db")
}

func DefaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "cadenced.sock")
	}
	cadenceDir := filepath.Join(home, ".cache", "cadence")
	_ = os.MkdirAll(cadenceDir, 0755)
	return filepath.Join(cadenceDir, "cadenced.sock")
}

func Default() Config {
	return Config{
		Audio: AudioConfig{SampleRate: 16000, Channels: 1},
		ASR: ASRConfig{
			Engine: "whisper", Model: "base.en",
			Language: "auto", Streaming: false,
		},
		Post: PostConfig{
			FillerRemoval: true,
			Punctuation:   true,
			Vocabulary:    true,
			ITN:           true,
		},
		Privacy: PrivacyConfig{
			NetworkMode: "local_only",
			Telemetry:   false,
		},
		Hotkey: HotkeyConfig{
			Shortcut: "Cmd+Shift+Space",
			Mode:     "push_to_talk",
		},
		IPC: IPCConfig{
			SocketPath: DefaultSocketPath(),
		},
		Storage: StorageConfig{
			DBPath: DefaultDBPath(),
		},
	}
}
