package audio

import (
	"context"
	"math"
	"sync"
	"time"
)

// SimulatedDriver generates synthetic audio samples for testing and non-hardware environments.
type SimulatedDriver struct {
	mu        sync.Mutex
	running   bool
	freq      float64       // Tone frequency (e.g. 440Hz, or 0 for silence)
	frameSize int           // Samples per emitted frame (e.g. 512)
	interval  time.Duration // Emission interval (e.g. 32ms)
	phase     float64
}

// NewSimulatedDriver creates a SimulatedDriver with default 440Hz tone.
func NewSimulatedDriver(frequency float64, frameSize int, interval time.Duration) *SimulatedDriver {
	if frameSize <= 0 {
		frameSize = 512
	}
	if interval <= 0 {
		interval = 32 * time.Millisecond
	}
	return &SimulatedDriver{
		freq:      frequency,
		frameSize: frameSize,
		interval:  interval,
	}
}

// StartStream begins emitting simulated PCM audio frames to the given channel.
func (s *SimulatedDriver) StartStream(ctx context.Context, sampleRate int, out chan<- []float32) error {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	if sampleRate <= 0 {
		sampleRate = DefaultSampleRate
	}

	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.mu.Lock()
				if !s.running {
					s.mu.Unlock()
					return
				}

				frame := make([]float32, s.frameSize)
				if s.freq > 0 {
					twoPi := 2.0 * math.Pi
					step := (s.freq * twoPi) / float64(sampleRate)
					for i := 0; i < s.frameSize; i++ {
						frame[i] = float32(math.Sin(s.phase) * 0.5) // 50% amplitude
						s.phase += step
						if s.phase >= twoPi {
							s.phase -= twoPi
						}
					}
				}
				s.mu.Unlock()

				select {
				case <-ctx.Done():
					return
				case out <- frame:
				}
			}
		}
	}()

	return nil
}

// StopStream halts the simulated stream.
func (s *SimulatedDriver) StopStream() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	return nil
}
