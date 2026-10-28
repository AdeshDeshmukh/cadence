package audio

import (
	"sync"
	"time"
)

const (
	// DefaultSampleRate is the default audio sample rate (16kHz).
	DefaultSampleRate = 16000

	// DefaultPreRollDuration is the default duration retained for pre-roll capture.
	DefaultPreRollDuration = 500 * time.Millisecond
)

// RingBuffer is a thread-safe circular buffer for float32 audio samples.
// It supports overwrite semantics on write overflow and efficient extraction
// of recent pre-roll samples.
type RingBuffer struct {
	mu       sync.RWMutex
	buf      []float32
	capacity int
	head     int // Next write position
	tail     int // Next read position
	count    int // Current number of valid samples stored
}

// NewRingBuffer allocates a new RingBuffer with the given capacity.
// If capacity <= 0, a sensible default of 16,000 samples (1 second at 16kHz) is used.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = DefaultSampleRate
	}
	return &RingBuffer{
		buf:      make([]float32, capacity),
		capacity: capacity,
	}
}

// NewPreRollBuffer allocates a RingBuffer sized for the specified duration and sample rate.
func NewPreRollBuffer(duration time.Duration, sampleRate int) *RingBuffer {
	if sampleRate <= 0 {
		sampleRate = DefaultSampleRate
	}
	if duration <= 0 {
		duration = DefaultPreRollDuration
	}
	samples := int(duration.Seconds() * float64(sampleRate))
	if samples <= 0 {
		samples = sampleRate / 2
	}
	return NewRingBuffer(samples)
}

// Capacity returns the maximum capacity of the ring buffer.
func (r *RingBuffer) Capacity() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.capacity
}

// Len returns the number of unread/stored samples currently in the buffer.
func (r *RingBuffer) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}

// Write writes audio samples into the ring buffer.
// If the number of incoming samples exceeds remaining capacity, oldest samples
// are overwritten so that the buffer always preserves the most recent samples.
// Returns the number of samples written.
func (r *RingBuffer) Write(samples []float32) int {
	if len(samples) == 0 {
		return 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	n := len(samples)

	// If incoming samples exceed or equal capacity, only the last capacity samples remain
	if n >= r.capacity {
		toCopy := samples[n-r.capacity:]
		copy(r.buf, toCopy)
		r.head = 0
		r.tail = 0
		r.count = r.capacity
		return n
	}

	// If adding n samples exceeds capacity, advance tail (overwrite oldest)
	if r.count+n > r.capacity {
		overwritten := (r.count + n) - r.capacity
		r.tail = (r.tail + overwritten) % r.capacity
		r.count = r.capacity
	} else {
		r.count += n
	}

	// Copy samples into circular buffer at head
	firstChunk := r.capacity - r.head
	if n <= firstChunk {
		copy(r.buf[r.head:r.head+n], samples)
	} else {
		copy(r.buf[r.head:], samples[:firstChunk])
		copy(r.buf[:n-firstChunk], samples[firstChunk:])
	}

	r.head = (r.head + n) % r.capacity
	return n
}

// WriteChunk writes an audio.Chunk's samples into the ring buffer.
func (r *RingBuffer) WriteChunk(chunk Chunk) int {
	return r.Write(chunk.Samples)
}

// ReadLatest returns a copy of the latest n samples written to the buffer.
// If n exceeds the number of available samples, all available samples are returned.
// Does NOT advance the read pointer (tail) or consume the samples.
func (r *RingBuffer) ReadLatest(n int) []float32 {
	if n <= 0 {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.count == 0 {
		return nil
	}

	if n > r.count {
		n = r.count
	}

	out := make([]float32, n)
	// The starting index for the latest n samples
	start := (r.head - n + r.capacity) % r.capacity

	firstChunk := r.capacity - start
	if n <= firstChunk {
		copy(out, r.buf[start:start+n])
	} else {
		copy(out[:firstChunk], r.buf[start:])
		copy(out[firstChunk:], r.buf[:n-firstChunk])
	}

	return out
}

// ReadLatestDuration returns the latest samples corresponding to the given duration
// at the specified sample rate.
func (r *RingBuffer) ReadLatestDuration(d time.Duration, sampleRate int) []float32 {
	if d <= 0 || sampleRate <= 0 {
		return nil
	}
	n := int(d.Seconds() * float64(sampleRate))
	return r.ReadLatest(n)
}

// Read reads up to len(dst) samples from the oldest unread position into dst.
// Advances the read pointer (tail) and decreases stored count.
// Returns the number of samples read.
func (r *RingBuffer) Read(dst []float32) int {
	if len(dst) == 0 {
		return 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 {
		return 0
	}

	toRead := len(dst)
	if toRead > r.count {
		toRead = r.count
	}

	firstChunk := r.capacity - r.tail
	if toRead <= firstChunk {
		copy(dst[:toRead], r.buf[r.tail:r.tail+toRead])
	} else {
		copy(dst[:firstChunk], r.buf[r.tail:])
		copy(dst[firstChunk:toRead], r.buf[:toRead-firstChunk])
	}

	r.tail = (r.tail + toRead) % r.capacity
	r.count -= toRead
	return toRead
}

// ReadAll reads and consumes all unread samples currently stored in the buffer.
func (r *RingBuffer) ReadAll() []float32 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 {
		return nil
	}

	out := make([]float32, r.count)
	toRead := r.count

	firstChunk := r.capacity - r.tail
	if toRead <= firstChunk {
		copy(out, r.buf[r.tail:r.tail+toRead])
	} else {
		copy(out[:firstChunk], r.buf[r.tail:])
		copy(out[firstChunk:], r.buf[:toRead-firstChunk])
	}

	r.tail = (r.tail + toRead) % r.capacity
	r.count = 0
	return out
}

// Peek returns up to n samples from the oldest unread position without advancing the read pointer.
func (r *RingBuffer) Peek(n int) []float32 {
	if n <= 0 {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.count == 0 {
		return nil
	}

	if n > r.count {
		n = r.count
	}

	out := make([]float32, n)
	firstChunk := r.capacity - r.tail
	if n <= firstChunk {
		copy(out, r.buf[r.tail:r.tail+n])
	} else {
		copy(out[:firstChunk], r.buf[r.tail:])
		copy(out[firstChunk:], r.buf[:n-firstChunk])
	}

	return out
}

// Reset clears the buffer, resetting head, tail, and count to zero.
func (r *RingBuffer) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.head = 0
	r.tail = 0
	r.count = 0
}
