package asr

import (
	"context"
	"fmt"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/worker"
)

type WhisperEngine struct {
	caller worker.Caller
}

func NewWhisperEngine(caller worker.Caller) *WhisperEngine {
	return &WhisperEngine{
		caller: caller,
	}
}

func (w *WhisperEngine) Name() string {
	return "whisper"
}

func (w *WhisperEngine) Transcribe(
	ctx context.Context,
	chunks []audio.Chunk,
) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	if len(chunks) == 0 {
		return Result{Final: true}, nil
	}

	sampleRate := 16000
	for _, chunk := range chunks {
		if chunk.SampleRate > 0 {
			if sampleRate == 16000 {
				sampleRate = chunk.SampleRate
			} else if chunk.SampleRate != sampleRate {
				return Result{}, fmt.Errorf("inconsistent sample rate across chunks: %d vs %d", chunk.SampleRate, sampleRate)
			}
		}
	}

	totalSamples := 0
	for _, chunk := range chunks {
		totalSamples += len(chunk.Samples)
	}

	samples := make([]float32, 0, totalSamples)
	for _, chunk := range chunks {
		samples = append(samples, chunk.Samples...)
	}

	if len(samples) == 0 {
		return Result{Final: true}, nil
	}

	if sampleRate != 16000 {
		samples = resampleLinear(samples, sampleRate, 16000)
		sampleRate = 16000
	}

	params := map[string]any{
		"audio":       samples,
		"sample_rate": sampleRate,
	}

	resp, err := w.caller.Call(ctx, "transcribe", params)
	if err != nil {
		return Result{}, fmt.Errorf("whisper transcribe call: %w", err)
	}

	var words []Word
	if rawWords, ok := resp.Get("words"); ok {
		if slice, ok := rawWords.([]any); ok {
			for _, item := range slice {
				if m, ok := item.(map[string]any); ok {
					var word Word
					if t, ok := m["text"].(string); ok {
						word.Text = t
					}
					if s, ok := m["start_ms"].(float64); ok {
						word.StartMs = int64(s)
					}
					if e, ok := m["end_ms"].(float64); ok {
						word.EndMs = int64(e)
					}
					if c, ok := m["confidence"].(float64); ok {
						word.Confidence = c
					}
					words = append(words, word)
				}
			}
		}
	}

	confidence := resp.GetFloat64("confidence")
	if confidence == 0 && resp.GetString("text") != "" {
		confidence = 1.0
	}

	return Result{
		Text:       resp.GetString("text"),
		Language:   resp.GetString("language"),
		Confidence: confidence,
		Words:      words,
		Final:      true,
	}, nil
}

func (w *WhisperEngine) Stream(
	ctx context.Context,
	in <-chan audio.Chunk,
) (<-chan PartialResult, error) {
	out := make(chan PartialResult, 4)

	go func() {
		defer close(out)

		var chunks []audio.Chunk
		bufferedSamples := 0
		const partialBufferThreshold = 16000 * 600 / 1000 // 600ms

		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-in:
				if !ok {
					if len(chunks) == 0 {
						return
					}
					res, err := w.Transcribe(ctx, chunks)
					if err != nil {
						return
					}
					select {
					case out <- PartialResult{
						Text:     res.Text,
						Language: res.Language,
						Final:    true,
					}:
					case <-ctx.Done():
					}
					return
				}

				chunks = append(chunks, chunk)
				bufferedSamples += len(chunk.Samples)
				if bufferedSamples >= partialBufferThreshold {
					if partialText, err := w.transcribePartial(ctx, chunks); err == nil && partialText != "" {
						select {
						case out <- PartialResult{
							Text:  partialText,
							Final: false,
						}:
						case <-ctx.Done():
							return
						}
					}
					bufferedSamples = 0
				}
			}
		}
	}()

	return out, nil
}

func (w *WhisperEngine) transcribePartial(ctx context.Context, chunks []audio.Chunk) (string, error) {
	if len(chunks) == 0 {
		return "", nil
	}
	totalSamples := 0
	for _, chunk := range chunks {
		totalSamples += len(chunk.Samples)
	}
	samples := make([]float32, 0, totalSamples)
	sampleRate := 16000
	for _, chunk := range chunks {
		if chunk.SampleRate > 0 {
			sampleRate = chunk.SampleRate
		}
		samples = append(samples, chunk.Samples...)
	}
	if len(samples) == 0 {
		return "", nil
	}
	if sampleRate != 16000 {
		samples = resampleLinear(samples, sampleRate, 16000)
		sampleRate = 16000
	}

	params := map[string]any{
		"audio":       samples,
		"sample_rate": sampleRate,
	}

	resp, err := w.caller.Call(ctx, "transcribe_partial", params)
	if err == nil && resp != nil && resp.OK {
		return resp.GetString("text"), nil
	}

	res, err := w.Transcribe(ctx, chunks)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

func resampleLinear(samples []float32, inRate, outRate int) []float32 {
	if inRate == outRate || inRate <= 0 || outRate <= 0 || len(samples) == 0 {
		return samples
	}
	ratio := float64(inRate) / float64(outRate)
	outLen := int(float64(len(samples)) / ratio)
	resampled := make([]float32, outLen)
	for i := 0; i < outLen; i++ {
		origPos := float64(i) * ratio
		idx := int(origPos)
		frac := float32(origPos - float64(idx))
		if idx+1 < len(samples) {
			resampled[i] = samples[idx]*(1-frac) + samples[idx+1]*frac
		} else if idx < len(samples) {
			resampled[i] = samples[idx]
		}
	}
	return resampled
}
