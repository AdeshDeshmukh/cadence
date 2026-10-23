package config

type Config struct {
	Audio   AudioConfig
	ASR     ASRConfig
	Post    PostConfig
	Privacy PrivacyConfig
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
}

type PrivacyConfig struct {
	NetworkMode string
	Telemetry   bool
}

func Default() Config {
	return Config{
		Audio: AudioConfig{SampleRate: 16000, Channels: 1},
		ASR: ASRConfig{
			Engine: "mock", Model: "whisper",
			Language: "auto", Streaming: false,
		},
		Post: PostConfig{
			FillerRemoval: true,
			Punctuation:   true,
			Vocabulary:    true,
		},
		Privacy: PrivacyConfig{
			NetworkMode: "local_only",
			Telemetry:   false,
		},
	}
}
