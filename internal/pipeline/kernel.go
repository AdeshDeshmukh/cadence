package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

type Kernel struct {
	capture audio.Capture
	asr     asr.Engine
	paste   paste.Paster
	process *postprocess.Pipeline
	store   storage.Store
}

func NewKernel(
	capture audio.Capture,
	engine asr.Engine,
	paster paste.Paster,
	process *postprocess.Pipeline,
	store storage.Store,
) *Kernel {
	return &Kernel{
		capture: capture, asr: engine, paste: paster,
		process: process, store: store,
	}
}

func (k *Kernel) Run(
	ctx context.Context,
	session *Session,
	pc postprocess.Context,
) error {
	session.SetState(Recording)

	chunksCh, err := k.capture.Start()
	if err != nil {
		session.SetState(Failed)
		return fmt.Errorf("capture: %w", err)
	}

	var chunks []audio.Chunk

	for {
		select {
		case <-ctx.Done():
			_ = k.capture.Stop()
			session.SetState(Cancelled)
			return ctx.Err()

		case chunk, ok := <-chunksCh:
			if !ok {
				goto transcribe
			}
			chunks = append(chunks, chunk)
		}
	}

transcribe:
	if err := k.capture.Stop(); err != nil {
		session.SetState(Failed)
		return fmt.Errorf("stop capture: %w", err)
	}

	session.SetState(Transcribing)

	result, err := k.asr.Transcribe(ctx, chunks)
	if err != nil {
		session.SetState(Failed)
		return fmt.Errorf("asr: %w", err)
	}

	session.RawText = result.Text
	session.SetState(Processing)

	session.FinalText = k.process.Apply(result.Text, pc)
	session.SetState(Pasting)

	if _, err := k.paste.Paste(session.FinalText); err != nil {
		_ = k.store.SaveTranscript(storage.Transcript{
			ID: session.ID, RawText: session.RawText,
			Text: session.FinalText, Language: result.Language,
			Mode: pc.Mode, Application: pc.Application,
			CreatedAt: time.Now(),
		})
		session.SetState(Failed)
		return fmt.Errorf("paste: %w", err)
	}

	if err := k.store.SaveTranscript(storage.Transcript{
		ID: session.ID, RawText: session.RawText,
		Text: session.FinalText, Language: result.Language,
		Mode: pc.Mode, Application: pc.Application,
		CreatedAt: time.Now(),
	}); err != nil {
		session.SetState(Failed)
		return fmt.Errorf("store: %w", err)
	}

	session.SetState(Complete)
	return nil
}
