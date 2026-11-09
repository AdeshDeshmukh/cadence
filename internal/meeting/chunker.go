package meeting

import (
	"fmt"
	"sync"
	"time"
)

// SegmentHandler is called when a rolling segment reaches target boundary.
type SegmentHandler func(seg Segment, samples []float32)

// ChunkerOptions configures the rolling segmentation parameters.
type ChunkerOptions struct {
	SegmentDuration time.Duration // default 15s
	OverlapDuration time.Duration // default 1s
	SampleRate      int           // default 16000
}

// Chunker segments continuous audio streams into rolling overlapping chunks.
type Chunker struct {
	mu          sync.Mutex
	meetingID   string
	sampleRate  int
	targetSize  int
	overlapSize int

	handler SegmentHandler
	wav     *WAVWriter

	buffer      []float32
	totalAudio  int64
	segIndex    int
	startOffset int64
}

// NewChunker initializes a rolling audio stream chunker.
func NewChunker(meetingID string, wav *WAVWriter, handler SegmentHandler, opts ChunkerOptions) *Chunker {
	sr := opts.SampleRate
	if sr <= 0 {
		sr = 16000
	}
	segDur := opts.SegmentDuration
	if segDur < 2*time.Second {
		segDur = 15 * time.Second
	}
	overlapDur := opts.OverlapDuration
	if overlapDur < 0 || overlapDur >= segDur {
		overlapDur = 1 * time.Second
	}

	targetSize := int(float64(sr) * segDur.Seconds())
	overlapSize := int(float64(sr) * overlapDur.Seconds())

	return &Chunker{
		meetingID:   meetingID,
		sampleRate:  sr,
		targetSize:  targetSize,
		overlapSize: overlapSize,
		handler:     handler,
		wav:         wav,
		buffer:      make([]float32, 0, targetSize*2),
	}
}

// Ingest processes an incoming slice of audio samples.
func (c *Chunker) Ingest(samples []float32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(samples) == 0 {
		return nil
	}

	// 1. Write to continuous disk WAV stream
	if c.wav != nil {
		if err := c.wav.WriteSamples(samples); err != nil {
			return err
		}
	}

	// 2. Accumulate in buffer
	c.buffer = append(c.buffer, samples...)
	c.totalAudio += int64(len(samples))

	// 3. Emit segments whenever buffer reaches targetSize
	for len(c.buffer) >= c.targetSize {
		chunkSamples := make([]float32, c.targetSize)
		copy(chunkSamples, c.buffer[:c.targetSize])

		startMs := int64(float64(c.startOffset) / float64(c.sampleRate) * 1000)
		endMs := int64(float64(c.startOffset+int64(c.targetSize)) / float64(c.sampleRate) * 1000)

		seg := Segment{
			ID:           fmt.Sprintf("%s-seg-%04d", c.meetingID, c.segIndex),
			MeetingID:    c.meetingID,
			SegmentIndex: c.segIndex,
			StartMs:      startMs,
			EndMs:        endMs,
			CreatedAt:    time.Now().UTC(),
		}

		c.segIndex++
		c.startOffset += int64(c.targetSize - c.overlapSize)

		// Retain overlap samples for next window boundary
		newBuf := make([]float32, len(c.buffer)-c.targetSize+c.overlapSize)
		copy(newBuf, c.buffer[c.targetSize-c.overlapSize:])
		c.buffer = newBuf

		if c.handler != nil {
			c.handler(seg, chunkSamples)
		}
	}

	return nil
}

// Flush emits any remaining residual audio in the buffer when the stream concludes.
func (c *Chunker) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.buffer) > c.sampleRate/2 { // at least 500ms of residual audio
		startMs := int64(float64(c.startOffset) / float64(c.sampleRate) * 1000)
		endMs := int64(float64(c.startOffset+int64(len(c.buffer))) / float64(c.sampleRate) * 1000)

		seg := Segment{
			ID:           fmt.Sprintf("%s-seg-%04d", c.meetingID, c.segIndex),
			MeetingID:    c.meetingID,
			SegmentIndex: c.segIndex,
			StartMs:      startMs,
			EndMs:        endMs,
			CreatedAt:    time.Now().UTC(),
		}

		residual := make([]float32, len(c.buffer))
		copy(residual, c.buffer)
		c.buffer = nil

		if c.handler != nil {
			c.handler(seg, residual)
		}
	}
}

// TotalSamples returns total ingested audio samples.
func (c *Chunker) TotalSamples() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totalAudio
}
