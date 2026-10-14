package vad

import (
	"math"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
)

type Simple struct {
	Threshold float32
	speaking  bool
}

func NewSimple(threshold float32) *Simple {
	return &Simple{Threshold: threshold}
}

func (v *Simple) Process(c audio.Chunk) Event {
	if len(c.Samples) == 0 {
		return Event{Type: Silence}
	}

	var sum float64
	for _, x := range c.Samples {
		sum += float64(x * x)
	}

	rms := math.Sqrt(sum / float64(len(c.Samples)))
	speaking := float32(rms) >= v.Threshold

	if speaking && !v.speaking {
		v.speaking = true
		return Event{Type: SpeechStart}
	}
	if !speaking && v.speaking {
		v.speaking = false
		return Event{Type: SpeechEnd}
	}
	if speaking {
		return Event{Type: SpeechStart}
	}
	return Event{Type: Silence}
}

func (v *Simple) Reset() {
	v.speaking = false
}
