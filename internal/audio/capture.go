package audio

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrAlreadyCapturing is returned when Start() is called while already capturing.
	ErrAlreadyCapturing = errors.New("audio: capture already in progress")

	// ErrNotCapturing is returned when Stop() is called while not capturing.
	ErrNotCapturing = errors.New("audio: capture is not running")

	// ErrDriverUnavailable is returned when no working stream driver is available.
	ErrDriverUnavailable = errors.New("audio: stream driver unavailable")
)

// StreamDriver defines the low-level hardware or simulated audio streaming contract.
type StreamDriver interface {
	StartStream(ctx context.Context, sampleRate int, out chan<- []float32) error
	StopStream() error
}

// Engine implements the Capture interface. It coordinates device selection,
// pre-roll ring buffering, and real-time audio chunk emission.
type Engine struct {
	mu              sync.Mutex
	deviceMgr       *Manager
	preRoll         *RingBuffer
	driver          StreamDriver
	sampleRate      int
	preRollDuration time.Duration

	activeChan  chan Chunk
	cancelFn    context.CancelFunc
	isCapturing bool
	wg          sync.WaitGroup
}

// EngineOption allows configuring Engine instances.
type EngineOption func(*Engine)

// WithSampleRate sets the audio sample rate (default 16000).
func WithSampleRate(rate int) EngineOption {
	return func(e *Engine) {
		if rate > 0 {
			e.sampleRate = rate
		}
	}
}

// WithPreRollDuration sets the pre-roll duration to retain (default 500ms).
func WithPreRollDuration(d time.Duration) EngineOption {
	return func(e *Engine) {
		if d > 0 {
			e.preRollDuration = d
		}
	}
}

// WithDriver sets a custom or simulated stream driver.
func WithDriver(driver StreamDriver) EngineOption {
	return func(e *Engine) {
		e.driver = driver
	}
}

// WithDeviceManager sets a custom device manager.
func WithDeviceManager(mgr *Manager) EngineOption {
	return func(e *Engine) {
		e.deviceMgr = mgr
	}
}

// NewEngine creates a new audio capture Engine.
func NewEngine(opts ...EngineOption) *Engine {
	e := &Engine{
		sampleRate:      DefaultSampleRate,
		preRollDuration: DefaultPreRollDuration,
	}

	for _, opt := range opts {
		opt(e)
	}

	if e.deviceMgr == nil {
		e.deviceMgr = NewManager(nil)
	}

	if e.preRoll == nil {
		e.preRoll = NewPreRollBuffer(e.preRollDuration, e.sampleRate)
	}

	if e.driver == nil {
		e.driver = NewDefaultStreamDriver()
	}

	return e
}

// Devices returns the available audio input devices.
func (e *Engine) Devices() ([]Device, error) {
	if e.deviceMgr == nil {
		return nil, ErrNoInputDevices
	}
	inputs := e.deviceMgr.InputDevices()
	if len(inputs) == 0 {
		return nil, ErrNoInputDevices
	}
	return inputs, nil
}

// DeviceManager returns the underlying device manager.
func (e *Engine) DeviceManager() *Manager {
	return e.deviceMgr
}

// RingBuffer returns the underlying pre-roll ring buffer.
func (e *Engine) RingBuffer() *RingBuffer {
	return e.preRoll
}

// IsCapturing returns whether audio capture is currently active.
func (e *Engine) IsCapturing() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.isCapturing
}

// Start begins streaming live audio. It extracts recent pre-roll samples,
// emits them as the initial chunk, and streams live incoming chunks.
func (e *Engine) Start() (<-chan Chunk, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isCapturing {
		return nil, ErrAlreadyCapturing
	}

	if e.driver == nil {
		return nil, ErrDriverUnavailable
	}

	outChan := make(chan Chunk, 128)
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFn = cancel
	e.activeChan = outChan
	e.isCapturing = true

	// 1. Inject Pre-Roll audio if available
	preRollSamples := e.preRoll.ReadLatestDuration(e.preRollDuration, e.sampleRate)
	if len(preRollSamples) > 0 {
		outChan <- Chunk{
			Samples:    preRollSamples,
			SampleRate: e.sampleRate,
			Timestamp:  time.Now().Add(-e.preRollDuration),
		}
	}

	// 2. Start hardware/driver stream
	rawChan := make(chan []float32, 64)
	if err := e.driver.StartStream(ctx, e.sampleRate, rawChan); err != nil {
		e.isCapturing = false
		cancel()
		close(outChan)
		return nil, err
	}

	// 3. Pump audio samples from driver to outChan and continuously update preRoll buffer
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer close(outChan)

		for {
			select {
			case <-ctx.Done():
				return
			case samples, ok := <-rawChan:
				if !ok {
					return
				}
				if len(samples) == 0 {
					continue
				}

				// Always update pre-roll ring buffer
				e.preRoll.Write(samples)

				// Emit live chunk
				outChan <- Chunk{
					Samples:    samples,
					SampleRate: e.sampleRate,
					Timestamp:  time.Now(),
				}
			}
		}
	}()

	return outChan, nil
}

// Stop terminates the active recording stream and closes the output channel.
func (e *Engine) Stop() error {
	e.mu.Lock()
	if !e.isCapturing {
		e.mu.Unlock()
		return ErrNotCapturing
	}

	e.isCapturing = false
	cancel := e.cancelFn
	e.cancelFn = nil
	driver := e.driver
	e.mu.Unlock()

	// Cancel context to stop stream pump
	if cancel != nil {
		cancel()
	}

	var driverErr error
	if driver != nil {
		driverErr = driver.StopStream()
	}

	e.wg.Wait()
	return driverErr
}
