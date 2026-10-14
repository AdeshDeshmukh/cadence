package vad

import "github.com/AdeshDeshmukh/cadence/internal/audio"

type EventType string

const (
	SpeechStart EventType = "speech_start"
	SpeechEnd   EventType = "speech_end"
	Silence     EventType = "silence"
)

type Event struct {
	Type EventType
}

type Detector interface {
	Process(audio.Chunk) Event
	Reset()
}
