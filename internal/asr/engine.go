package asr

import (
	"context"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
)

type Word struct {
	Text       string
	StartMs    int64
	EndMs      int64
	Confidence float64
}

type Result struct {
	Text       string
	Language   string
	Confidence float64
	Words      []Word
	Final      bool
}

type PartialResult struct {
	Text     string
	Language string
	Final    bool
}

type Engine interface {
	Name() string
	Transcribe(context.Context, []audio.Chunk) (Result, error)
	Stream(context.Context, <-chan audio.Chunk) (<-chan PartialResult, error)
}
