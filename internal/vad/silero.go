package vad

import (
	"context"
	"sync"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/worker"
)

// SileroConfig configures neural voice activity detection parameters.
type SileroConfig struct {
	Threshold         float64
	MinSpeechDuration time.Duration
	HangoverDuration  time.Duration
}

// DefaultSileroConfig returns default neural VAD configuration.
func DefaultSileroConfig() SileroConfig {
	return SileroConfig{
		Threshold:         0.5,
		MinSpeechDuration: 80 * time.Millisecond,
		HangoverDuration:  500 * time.Millisecond,
	}
}

// SileroOption allows configuring SileroDetector.
type SileroOption func(*SileroDetector)

// WithSileroThreshold sets detection confidence threshold (0.0 - 1.0).
func WithSileroThreshold(t float64) SileroOption {
	return func(s *SileroDetector) {
		if t > 0 && t <= 1.0 {
			s.config.Threshold = t
		}
	}
}

// WithSileroHangover sets the hangover debounce duration.
func WithSileroHangover(d time.Duration) SileroOption {
	return func(s *SileroDetector) {
		if d > 0 {
			s.config.HangoverDuration = d
		}
	}
}

// SileroDetector implements vad.Detector backed by a neural Silero VAD subprocess worker.
type SileroDetector struct {
	mu     sync.Mutex
	caller worker.Caller
	config SileroConfig

	isSpeaking     bool
	speechDuration time.Duration
	silenceElapsed time.Duration
}

// NewSileroDetector returns a new neural VAD detector.
func NewSileroDetector(caller worker.Caller, opts ...SileroOption) *SileroDetector {
	s := &SileroDetector{
		caller: caller,
		config: DefaultSileroConfig(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Process evaluates a chunk of audio through the neural worker and emits state change events.
func (s *SileroDetector) Process(chunk audio.Chunk) Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(chunk.Samples) == 0 {
		return Event{Type: Silence}
	}

	sampleRate := chunk.SampleRate
	if sampleRate <= 0 {
		sampleRate = 16000
	}

	chunkDur := time.Duration(float64(len(chunk.Samples)) / float64(sampleRate) * float64(time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	params := map[string]any{
		"audio":       chunk.Samples,
		"sample_rate": sampleRate,
		"threshold":   s.config.Threshold,
	}

	resp, err := s.caller.Call(ctx, "detect", params)
	if err != nil || resp == nil || !resp.OK {
		// On worker call error, preserve current speaking state without false onset
		if s.isSpeaking {
			return Event{Type: Silence}
		}
		return Event{Type: Silence}
	}

	val, ok := resp.Get("is_speech")
	isSpeech := false
	if ok {
		if b, ok := val.(bool); ok {
			isSpeech = b
		}
	}

	if isSpeech {
		s.silenceElapsed = 0
		s.speechDuration += chunkDur

		if !s.isSpeaking && s.speechDuration >= s.config.MinSpeechDuration {
			s.isSpeaking = true
			return Event{Type: SpeechStart}
		}
		return Event{Type: Silence}
	}

	// Silence detected
	s.speechDuration = 0
	if s.isSpeaking {
		s.silenceElapsed += chunkDur
		if s.silenceElapsed >= s.config.HangoverDuration {
			s.isSpeaking = false
			s.silenceElapsed = 0
			return Event{Type: SpeechEnd}
		}
	}

	return Event{Type: Silence}
}

// Reset clears state and instructs the neural worker to reset recurrent state.
func (s *SileroDetector) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isSpeaking = false
	s.speechDuration = 0
	s.silenceElapsed = 0

	if s.caller != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = s.caller.Call(ctx, "reset", nil)
	}
}

// IsSpeaking returns whether speech is currently active.
func (s *SileroDetector) IsSpeaking() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isSpeaking
}
