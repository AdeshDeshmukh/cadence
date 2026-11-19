package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
)

// DefaultPCMChunkSamples is the default number of audio samples per emitted float32 chunk (32ms at 16kHz).
const DefaultPCMChunkSamples = 512

// PCM16ToFloat32 converts a byte buffer containing little-endian 16-bit signed PCM into normalized float32 samples.
func PCM16ToFloat32(pcm []byte) []float32 {
	numSamples := len(pcm) / 2
	if numSamples == 0 {
		return nil
	}
	out := make([]float32, numSamples)
	for i := 0; i < numSamples; i++ {
		raw := int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2]))
		out[i] = float32(raw) / 32768.0
	}
	return out
}

// Float32ToPCM16 converts normalized float32 audio samples into little-endian 16-bit signed PCM bytes.
func Float32ToPCM16(samples []float32) []byte {
	out := make([]byte, len(samples)*2)
	for i, s := range samples {
		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		val := int16(s * 32767.0)
		binary.LittleEndian.PutUint16(out[i*2:i*2+2], uint16(val))
	}
	return out
}

// ReadPCM16Stream reads 16-bit linear PCM from an io.Reader, converts samples to float32,
// and streams them into the output channel in chunks of chunkSamples.
func ReadPCM16Stream(reader io.Reader, ctx context.Context, chunkSamples int, out chan<- []float32) error {
	if chunkSamples <= 0 {
		chunkSamples = DefaultPCMChunkSamples
	}
	byteSize := chunkSamples * 2
	buf := make([]byte, byteSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := io.ReadFull(reader, buf)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) {
				if n >= 2 {
					samples := PCM16ToFloat32(buf[:n-(n%2)])
					if len(samples) > 0 {
						select {
						case out <- samples:
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}
				return nil
			}
			return err
		}

		samples := PCM16ToFloat32(buf[:n])
		if len(samples) > 0 {
			select {
			case out <- samples:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}
