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
	ctx context.Context,
	_ []audio.Chunk,
) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return Result{
		Text: m.Text, Language: "en",
		Confidence: 1, Final: true,
	}, nil
}

func (m MockEngine) Stream(
	ctx context.Context,
	in <-chan audio.Chunk,
) (<-chan PartialResult, error) {
	out := make(chan PartialResult, 2)

	go func() {
		defer close(out)

		// Drain upstream audio chunks so producer doesn't block
		drainDone := make(chan struct{})
		go func() {
			defer close(drainDone)
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-in:
					if !ok {
						return
					}
				}
			}
		}()

		if err := ctx.Err(); err != nil {
			return
		}

		select {
		case out <- PartialResult{Text: m.Text, Language: "en"}:
		case <-ctx.Done():
			return
		}

		if err := ctx.Err(); err != nil {
			return
		}

		select {
		case out <- PartialResult{Text: m.Text, Language: "en", Final: true}:
		case <-ctx.Done():
			return
		}

		<-drainDone
	}()

	return out, nil
}
