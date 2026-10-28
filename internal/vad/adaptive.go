package vad

import (
	"math"
	"sync"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
)

// Default adaptive VAD timing constants.
const (
	DefaultMinSpeechDuration   = 80 * time.Millisecond
	DefaultHangoverDuration    = 600 * time.Millisecond
	DefaultSNRRatio            = 2.2
	DefaultNoiseAdaptationRate = 0.05
	DefaultInitialNoiseFloor   = 0.005
	DefaultMinThreshold        = 0.015
)

// AdaptiveConfig configures the dynamic noise-floor tracking VAD.
type AdaptiveConfig struct {
	// MinSpeechDuration is the minimum duration required to confirm speech onset (filters impulse clicks).
	MinSpeechDuration time.Duration

	// HangoverDuration is how long silence must persist before triggering SpeechEnd (prevents mid-sentence cut-offs).
	HangoverDuration time.Duration

	// SNRRatio is the ratio above noise floor required to qualify as speech.
	SNRRatio float64

	// NoiseAdaptationRate is the exponential moving average weight for background noise calibration.
	NoiseAdaptationRate float64

	// InitialNoiseFloor sets the baseline noise floor estimate.
	InitialNoiseFloor float64

	// MinThreshold is the absolute floor for speech threshold even in dead silent rooms.
	MinThreshold float64
}

// DefaultAdaptiveConfig returns production-tuned defaults.
func DefaultAdaptiveConfig() AdaptiveConfig {
	return AdaptiveConfig{
		MinSpeechDuration:   DefaultMinSpeechDuration,
		HangoverDuration:    DefaultHangoverDuration,
		SNRRatio:            DefaultSNRRatio,
		NoiseAdaptationRate: DefaultNoiseAdaptationRate,
		InitialNoiseFloor:   DefaultInitialNoiseFloor,
		MinThreshold:        DefaultMinThreshold,
	}
}

// AdaptiveDetector implements vad.Detector using dynamic noise-floor tracking and hangover debouncing.
type AdaptiveDetector struct {
	mu     sync.RWMutex
	config AdaptiveConfig

	noiseFloor      float64
	isSpeaking      bool
	speechDuration  time.Duration
	silenceDuration time.Duration
	lastRMS         float64
}

// NewAdaptive creates an AdaptiveDetector with default configuration.
func NewAdaptive() *AdaptiveDetector {
	return NewAdaptiveWithConfig(DefaultAdaptiveConfig())
}

// NewAdaptiveWithConfig creates an AdaptiveDetector with custom parameters.
func NewAdaptiveWithConfig(cfg AdaptiveConfig) *AdaptiveDetector {
	if cfg.MinSpeechDuration <= 0 {
		cfg.MinSpeechDuration = DefaultMinSpeechDuration
	}
	if cfg.HangoverDuration <= 0 {
		cfg.HangoverDuration = DefaultHangoverDuration
	}
	if cfg.SNRRatio <= 1.0 {
		cfg.SNRRatio = DefaultSNRRatio
	}
	if cfg.NoiseAdaptationRate <= 0 || cfg.NoiseAdaptationRate > 1.0 {
		cfg.NoiseAdaptationRate = DefaultNoiseAdaptationRate
	}
	if cfg.InitialNoiseFloor <= 0 {
		cfg.InitialNoiseFloor = DefaultInitialNoiseFloor
	}
	if cfg.MinThreshold <= 0 {
		cfg.MinThreshold = DefaultMinThreshold
	}

	return &AdaptiveDetector{
		config:     cfg,
		noiseFloor: cfg.InitialNoiseFloor,
	}
}

// Process evaluates an audio chunk and transitions the speech state.
func (v *AdaptiveDetector) Process(c audio.Chunk) Event {
	v.mu.Lock()
	defer v.mu.Unlock()

	duration := c.Duration()
	if duration <= 0 && c.SampleRate > 0 && len(c.Samples) > 0 {
		duration = time.Duration(float64(len(c.Samples)) / float64(c.SampleRate) * float64(time.Second))
	}
	if duration <= 0 {
		duration = 20 * time.Millisecond // sensible fallback for zero sample chunks
	}

	// Empty chunk handling
	if len(c.Samples) == 0 {
		if v.isSpeaking {
			v.silenceDuration += duration
			if v.silenceDuration >= v.config.HangoverDuration {
				v.isSpeaking = false
				v.silenceDuration = 0
				v.speechDuration = 0
				return Event{Type: SpeechEnd}
			}
			return Event{Type: SpeechStart}
		}
		return Event{Type: Silence}
	}

	// 1. Calculate RMS energy
	var sumSquares float64
	for _, s := range c.Samples {
		sumSquares += float64(s * s)
	}
	rms := math.Sqrt(sumSquares / float64(len(c.Samples)))
	v.lastRMS = rms

	// 2. Dynamic threshold calculation
	dynamicThreshold := math.Max(v.noiseFloor*v.config.SNRRatio, v.config.MinThreshold)
	isLoud := rms >= dynamicThreshold

	// 3. State machine transitions
	if isLoud {
		v.silenceDuration = 0

		if !v.isSpeaking {
			v.speechDuration += duration
			if v.speechDuration >= v.config.MinSpeechDuration {
				v.isSpeaking = true
				return Event{Type: SpeechStart}
			}
			// Transient loud impulse not yet confirmed as speech
			return Event{Type: Silence}
		}

		// Continues speech
		return Event{Type: SpeechStart}
	}

	// Below speech threshold
	v.speechDuration = 0

	if !v.isSpeaking {
		// Update ambient noise floor estimate during confirmed silence
		v.noiseFloor = (1.0-v.config.NoiseAdaptationRate)*v.noiseFloor + v.config.NoiseAdaptationRate*rms
		return Event{Type: Silence}
	}

	// Currently speaking, but encounter silence: check hangover debouncing
	v.silenceDuration += duration
	if v.silenceDuration >= v.config.HangoverDuration {
		v.isSpeaking = false
		v.silenceDuration = 0
		return Event{Type: SpeechEnd}
	}

	// Within hangover period: preserve speaking state
	return Event{Type: SpeechStart}
}

// Reset clears speech timers and restores initial noise floor.
func (v *AdaptiveDetector) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.isSpeaking = false
	v.speechDuration = 0
	v.silenceDuration = 0
	v.noiseFloor = v.config.InitialNoiseFloor
	v.lastRMS = 0
}

// NoiseFloor returns the currently estimated background noise floor.
func (v *AdaptiveDetector) NoiseFloor() float64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.noiseFloor
}

// IsSpeaking returns whether the detector currently considers the user to be speaking.
func (v *AdaptiveDetector) IsSpeaking() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.isSpeaking
}

// LastRMS returns the RMS energy of the most recently processed audio chunk.
func (v *AdaptiveDetector) LastRMS() float64 {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.lastRMS
}
