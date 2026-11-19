package audio

import (
	"bytes"
	"context"
	"math"
	"testing"
	"time"
)

func TestPCM_ConversionRoundTrip(t *testing.T) {
	// Generate a 1kHz sine wave at 16kHz sample rate
	sampleCount := 512
	original := make([]float32, sampleCount)
	for i := 0; i < sampleCount; i++ {
		tVal := float64(i) / 16000.0
		original[i] = float32(math.Sin(2.0 * math.Pi * 1000.0 * tVal))
	}

	pcmBytes := Float32ToPCM16(original)
	if len(pcmBytes) != sampleCount*2 {
		t.Fatalf("expected %d bytes, got %d", sampleCount*2, len(pcmBytes))
	}

	recovered := PCM16ToFloat32(pcmBytes)
	if len(recovered) != sampleCount {
		t.Fatalf("expected %d float samples, got %d", sampleCount, len(recovered))
	}

	// Verify quantization error is minimal (within 1/32768 approx 0.0001)
	for i := 0; i < sampleCount; i++ {
		diff := math.Abs(float64(original[i] - recovered[i]))
		if diff > 0.001 {
			t.Errorf("sample %d: expected ~%f, got %f (diff: %f)", i, original[i], recovered[i], diff)
		}
	}
}

func TestPCM_Clamping(t *testing.T) {
	outOfBounds := []float32{-2.5, 2.5, 0.0}
	pcmBytes := Float32ToPCM16(outOfBounds)
	recovered := PCM16ToFloat32(pcmBytes)

	if recovered[0] < -1.0 || recovered[0] > -0.999 {
		t.Errorf("expected -1.0 clamped sample, got %f", recovered[0])
	}
	if recovered[1] > 1.0 || recovered[1] < 0.999 {
		t.Errorf("expected 1.0 clamped sample, got %f", recovered[1])
	}
	if recovered[2] != 0.0 {
		t.Errorf("expected 0.0 sample, got %f", recovered[2])
	}
}

func TestPCM_ReadPCM16Stream(t *testing.T) {
	// Create 3 chunks of 256 samples
	chunkSamples := 256
	totalSamples := chunkSamples * 3
	synthetic := make([]float32, totalSamples)
	for i := range synthetic {
		synthetic[i] = 0.5
	}

	pcmData := Float32ToPCM16(synthetic)
	reader := bytes.NewReader(pcmData)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	outCh := make(chan []float32, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- ReadPCM16Stream(reader, ctx, chunkSamples, outCh)
	}()

	var receivedSamples int
	var chunkCount int

readLoop:
	for {
		select {
		case chunk, ok := <-outCh:
			if !ok {
				break readLoop
			}
			receivedSamples += len(chunk)
			chunkCount++
			if receivedSamples >= totalSamples {
				break readLoop
			}
		case err := <-errCh:
			if err != nil {
				t.Fatalf("ReadPCM16Stream returned unexpected error: %v", err)
			}
			// Drain remaining chunks in outCh
			for {
				select {
				case chunk, ok := <-outCh:
					if !ok {
						break readLoop
					}
					receivedSamples += len(chunk)
					chunkCount++
				default:
					break readLoop
				}
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timed out waiting for stream chunks")
		}
	}

	if receivedSamples != totalSamples {
		t.Fatalf("expected %d total samples, got %d", totalSamples, receivedSamples)
	}
	if chunkCount != 3 {
		t.Fatalf("expected 3 chunks of %d, got %d chunks", chunkSamples, chunkCount)
	}
}
