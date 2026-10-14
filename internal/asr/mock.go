package asr

import (
	"context"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
)

type MockEngine struct {
	Text string
}

func (m MockEngine) Name() string { return "mock" }

func (m MockEngine) Transcribe(
	_ context.Context,
	_ []audio.Chunk,
) (Result, error) {
	return Result{
		Text: m.Text, Language: "en",
		Confidence: 1, Final: true,
	}, nil
}

func (m MockEngine) Stream(
	ctx context.Context,
	_ <-chan audio.Chunk,
) (<-chan PartialResult, error) {
	out := make(chan PartialResult, 2)
	go func() {
		defer close(out)
		select {
		case out <- PartialResult{Text: m.Text, Language: "en"}:
		case <-ctx.Done():
			return
		}
		select {
		case out <- PartialResult{Text: m.Text, Language: "en", Final: true}:
		case <-ctx.Done():
		}
	}()
	return out, nil
}
