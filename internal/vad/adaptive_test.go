package vad

import (
	"sync"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
)

func makeTestChunk(amplitude float32, duration time.Duration, sampleRate int) audio.Chunk {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	numSamples := int(duration.Seconds() * float64(sampleRate))
	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = amplitude
	}
	return audio.Chunk{
		Samples:    samples,
		SampleRate: sampleRate,
		Timestamp:  time.Now(),
	}
}

func TestAdaptiveVAD_ConfigDefaults(t *testing.T) {
	v := NewAdaptive()
	if v.NoiseFloor() != DefaultInitialNoiseFloor {
		t.Fatalf("expected initial noise floor %f, got %f", DefaultInitialNoiseFloor, v.NoiseFloor())
	}
	if v.IsSpeaking() {
		t.Fatalf("expected not speaking initially")
	}

	// Negative config values should fallback to defaults
	vCustom := NewAdaptiveWithConfig(AdaptiveConfig{
		MinSpeechDuration:   -1,
		HangoverDuration:    -1,
		SNRRatio:            0.5,
		NoiseAdaptationRate: -1,
		InitialNoiseFloor:   -1,
		MinThreshold:        -1,
	})
	if vCustom.config.MinSpeechDuration != DefaultMinSpeechDuration {
		t.Fatalf("expected fallback min speech duration")
	}
}

func TestAdaptiveVAD_ImpulseRejection(t *testing.T) {
	// Configure 100ms minimum speech duration
	v := NewAdaptiveWithConfig(AdaptiveConfig{
		MinSpeechDuration: 100 * time.Millisecond,
		InitialNoiseFloor: 0.005,
		MinThreshold:      0.02,
	})

	// Send a 30ms loud click (e.g. keyboard tap, pen drop)
	click := makeTestChunk(0.2, 30*time.Millisecond, 16000)
	ev := v.Process(click)

	if ev.Type != Silence {
		t.Fatalf("expected 30ms impulse click to be filtered as Silence, got %s", ev.Type)
	}
	if v.IsSpeaking() {
		t.Fatalf("expected IsSpeaking() to be false after impulse click")
	}
}

func TestAdaptiveVAD_SustainedSpeechAndHangover(t *testing.T) {
	v := NewAdaptiveWithConfig(AdaptiveConfig{
		MinSpeechDuration: 60 * time.Millisecond,
		HangoverDuration:  300 * time.Millisecond,
		MinThreshold:      0.02,
		SNRRatio:          2.0,
	})

	// 1. Send sustained speech for 100ms (above 60ms min duration)
	chunkSpeech := makeTestChunk(0.15, 100*time.Millisecond, 16000)
	ev := v.Process(chunkSpeech)

	if ev.Type != SpeechStart {
		t.Fatalf("expected SpeechStart for sustained speech, got %s", ev.Type)
	}
	if !v.IsSpeaking() {
		t.Fatalf("expected IsSpeaking() to be true")
	}

	// 2. Mid-sentence pause: send 150ms of silence (less than 300ms hangover)
	chunkPause := makeTestChunk(0.001, 150*time.Millisecond, 16000)
	evPause := v.Process(chunkPause)

	if evPause.Type != SpeechStart {
		t.Fatalf("expected SpeechStart preserved during hangover pause, got %s", evPause.Type)
	}
	if !v.IsSpeaking() {
		t.Fatalf("expected IsSpeaking() to stay true during hangover pause")
	}

	// 3. Resumed speech: user says next word
	evResume := v.Process(chunkSpeech)
	if evResume.Type != SpeechStart {
		t.Fatalf("expected SpeechStart when speech resumes, got %s", evResume.Type)
	}

	// 4. Final silence: send another 350ms of silence (exceeds 300ms hangover!)
	chunkFinalSilence := makeTestChunk(0.001, 350*time.Millisecond, 16000)
	evEnd := v.Process(chunkFinalSilence)

	if evEnd.Type != SpeechEnd {
		t.Fatalf("expected SpeechEnd when hangover exceeded, got %s", evEnd.Type)
	}
	if v.IsSpeaking() {
		t.Fatalf("expected IsSpeaking() to become false after SpeechEnd")
	}

	// 5. Continued silence after end
	evSilence := v.Process(chunkPause)
	if evSilence.Type != Silence {
		t.Fatalf("expected Silence after SpeechEnd, got %s", evSilence.Type)
	}
}

func TestAdaptiveVAD_NoiseFloorAdaptation(t *testing.T) {
	v := NewAdaptiveWithConfig(AdaptiveConfig{
		InitialNoiseFloor:   0.002,
		NoiseAdaptationRate: 0.2, // fast adaptation for test
		MinThreshold:        0.05,
	})

	initialFloor := v.NoiseFloor()

	// Send steady background hum of 0.015 (below min speech threshold of 0.05)
	hum := makeTestChunk(0.015, 50*time.Millisecond, 16000)
	for i := 0; i < 10; i++ {
		ev := v.Process(hum)
		if ev.Type != Silence {
			t.Fatalf("expected background hum to register as silence, got %s", ev.Type)
		}
	}

	// Noise floor should have moved upward toward 0.015
	adaptedFloor := v.NoiseFloor()
	if adaptedFloor <= initialFloor {
		t.Fatalf("expected noise floor to adapt upward, initial: %f, adapted: %f", initialFloor, adaptedFloor)
	}
}

func TestAdaptiveVAD_EmptyChunkAndReset(t *testing.T) {
	v := NewAdaptive()

	// Empty chunk while idle
	empty := audio.Chunk{}
	if ev := v.Process(empty); ev.Type != Silence {
		t.Fatalf("expected Silence for empty chunk when idle, got %s", ev.Type)
	}

	// Trigger speech
	v.Process(makeTestChunk(0.3, 200*time.Millisecond, 16000))
	if !v.IsSpeaking() {
		t.Fatalf("expected speaking")
	}

	// Empty chunk while speaking accumulates silence until SpeechEnd
	for i := 0; i < 35; i++ {
		ev := v.Process(empty)
		if ev.Type == SpeechEnd {
			break
		}
	}
	if v.IsSpeaking() {
		t.Fatalf("expected speaking to end after accumulated empty chunks")
	}

	// Reset check
	v.Process(makeTestChunk(0.3, 200*time.Millisecond, 16000))
	v.Reset()
	if v.IsSpeaking() {
		t.Fatalf("expected not speaking after Reset()")
	}
	if v.NoiseFloor() != DefaultInitialNoiseFloor {
		t.Fatalf("expected noise floor reset")
	}
}

func TestAdaptiveVAD_ConcurrentRace(t *testing.T) {
	v := NewAdaptive()
	var wg sync.WaitGroup

	numGoroutines := 8
	iterations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			speechChunk := makeTestChunk(0.2, 20*time.Millisecond, 16000)
			silenceChunk := makeTestChunk(0.001, 20*time.Millisecond, 16000)

			for j := 0; j < iterations; j++ {
				if j%2 == 0 {
					_ = v.Process(speechChunk)
				} else {
					_ = v.Process(silenceChunk)
				}
				_ = v.IsSpeaking()
				_ = v.NoiseFloor()
				_ = v.LastRMS()
				if j%50 == 0 {
					v.Reset()
				}
			}
		}(i)
	}

	wg.Wait()
}
