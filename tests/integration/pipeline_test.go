package integration_test

import (
	"context"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/pipeline"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestPipeline(t *testing.T) {
	capture := audio.NewMockCapture([]audio.Chunk{{
		Samples:    make([]float32, 320),
		SampleRate: 16000,
	}})

	clipboard := paste.NewClipboard()

	processor := postprocess.New(
		postprocess.NewFillerRemoval(),
		postprocess.NewPunctuation(),
	)

	store := storage.NewMemory()

	kernel := pipeline.NewKernel(
		capture,
		asr.MockEngine{Text: "um hello comma world"},
		clipboard,
		processor,
		store,
	)

	session := pipeline.NewSession("test")

	if err := kernel.Run(
		context.Background(),
		session,
		postprocess.Context{Language: "en", Mode: "general"},
	); err != nil {
		t.Fatal(err)
	}

	if got := clipboard.Text(); got != "Hello, world." {
		t.Fatalf("got %q", got)
	}

	if session.GetState() != pipeline.Complete {
		t.Fatalf("state=%s", session.GetState())
	}
}
