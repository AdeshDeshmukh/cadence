package meeting

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// WAVWriter handles streaming raw float32 audio samples directly to a standard 16-bit PCM WAV file.
type WAVWriter struct {
	mu         sync.Mutex
	file       *os.File
	path       string
	sampleRate int
	numSamples int64
	closed     bool
}

// NewWAVWriter creates and initializes a WAV file with header placeholders for streaming.
func NewWAVWriter(path string, sampleRate int) (*WAVWriter, error) {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create audio dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create wav file: %w", err)
	}

	w := &WAVWriter{
		file:       f,
		path:       path,
		sampleRate: sampleRate,
	}

	// Write 44-byte placeholder header
	if err := w.writeHeader(0); err != nil {
		_ = f.Close()
		return nil, err
	}

	return w, nil
}

// WriteSamples converts float32 samples to 16-bit linear PCM and writes them to the file.
func (w *WAVWriter) WriteSamples(samples []float32) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return errors.New("wav writer is closed")
	}
	if len(samples) == 0 {
		return nil
	}

	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		// Clamp float32 between -1.0 and 1.0
		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		val := int16(math.Round(float64(s * 32767.0)))
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(val))
	}

	n, err := w.file.Write(buf)
	if err != nil {
		return err
	}
	w.numSamples += int64(n / 2)
	return nil
}

// Close finalizes the WAV file header with total byte counts and closes the file descriptor.
func (w *WAVWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	dataBytes := uint32(w.numSamples * 2)

	// Seek to beginning and rewrite header with finalized sizes
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		_ = w.file.Close()
		return err
	}

	if err := w.writeHeader(dataBytes); err != nil {
		_ = w.file.Close()
		return err
	}

	return w.file.Close()
}

// writeHeader writes the 44-byte canonical RIFF/WAVE header.
func (w *WAVWriter) writeHeader(dataSize uint32) error {
	header := make([]byte, 44)

	// ChunkID "RIFF"
	copy(header[0:4], []byte("RIFF"))
	// ChunkSize = 36 + dataSize
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)
	// Format "WAVE"
	copy(header[8:12], []byte("WAVE"))

	// Subchunk1ID "fmt "
	copy(header[12:16], []byte("fmt "))
	// Subchunk1Size = 16 for PCM
	binary.LittleEndian.PutUint32(header[16:20], 16)
	// AudioFormat = 1 (linear PCM)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	// NumChannels = 1 (mono)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	// SampleRate
	binary.LittleEndian.PutUint32(header[24:28], uint32(w.sampleRate))
	// ByteRate = SampleRate * NumChannels * BitsPerSample/8
	binary.LittleEndian.PutUint32(header[28:32], uint32(w.sampleRate*2))
	// BlockAlign = NumChannels * BitsPerSample/8
	binary.LittleEndian.PutUint16(header[32:34], 2)
	// BitsPerSample = 16
	binary.LittleEndian.PutUint16(header[34:36], 16)

	// Subchunk2ID "data"
	copy(header[36:40], []byte("data"))
	// Subchunk2Size = dataSize
	binary.LittleEndian.PutUint32(header[40:44], dataSize)

	_, err := w.file.Write(header)
	return err
}

// Path returns the path of the WAV file.
func (w *WAVWriter) Path() string {
	return w.path
}

// NumSamples returns the count of samples written so far.
func (w *WAVWriter) NumSamples() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.numSamples
}
