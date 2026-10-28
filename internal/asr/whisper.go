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

	sampleRate := chunks[0].SampleRate
	if sampleRate <= 0 {
		sampleRate = 16000
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
	out := make(chan PartialResult, 2)

	go func() {
		defer close(out)

		var chunks []audio.Chunk
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
			}
		}
	}()

	return out, nil
}
