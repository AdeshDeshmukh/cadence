package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	_ = k.store.SaveRecovery(storage.SessionRecovery{
		ID:        session.ID,
		State:     "uncommitted",
		CreatedAt: session.StartedAt,
		UpdatedAt: time.Now(),
	})

	chunksCh, err := k.capture.Start()
	if err != nil {
		session.SetState(Failed)
		_ = k.store.SaveRecovery(storage.SessionRecovery{
			ID:        session.ID,
			State:     "failed",
			Error:     err.Error(),
			CreatedAt: session.StartedAt,
			UpdatedAt: time.Now(),
		})
		return fmt.Errorf("capture: %w", err)
	}

	streamIn := make(chan audio.Chunk, 16)
	var streamWg sync.WaitGroup

	partialsCh, streamErr := k.asr.Stream(ctx, streamIn)
	if streamErr == nil && partialsCh != nil {
		streamWg.Add(1)
		go func() {
			defer streamWg.Done()
			for partial := range partialsCh {
				session.UpdatePartial(partial.Text)
			}
		}()
	}

	var chunks []audio.Chunk

	for {
		select {
		case <-ctx.Done():
			close(streamIn)
			streamWg.Wait()
			_ = k.capture.Stop()
			session.SetState(Cancelled)

			totalSamples := 0
			for _, chunk := range chunks {
				totalSamples += len(chunk.Samples)
			}
			recText := session.GetPartial()
			if recText == "" && len(chunks) > 0 {
				quickCtx, quickCancel := context.WithTimeout(context.Background(), 2*time.Second)
				if res, err := k.asr.Transcribe(quickCtx, chunks); err == nil && res.Text != "" {
					recText = res.Text
					session.RawText = res.Text
				}
				quickCancel()
			}
			_ = k.store.SaveRecovery(storage.SessionRecovery{
				ID:           session.ID,
				State:        "cancelled",
				PartialText:  session.GetPartial(),
				RawText:      session.RawText,
				Text:         recText,
				AudioSamples: totalSamples,
				CreatedAt:    session.StartedAt,
				UpdatedAt:    time.Now(),
			})
			return ctx.Err()

		case chunk, ok := <-chunksCh:
			if !ok {
				close(streamIn)
				streamWg.Wait()
				goto transcribe
			}
			chunks = append(chunks, chunk)
			if partialsCh != nil {
				select {
				case streamIn <- chunk:
				default:
				}
			}
		}
	}

transcribe:
	totalSamples := 0
	for _, chunk := range chunks {
		totalSamples += len(chunk.Samples)
	}

	if err := k.capture.Stop(); err != nil && !errors.Is(err, audio.ErrNotCapturing) {
		session.SetState(Failed)
		_ = k.store.SaveRecovery(storage.SessionRecovery{
			ID:           session.ID,
			State:        "failed",
			PartialText:  session.GetPartial(),
			AudioSamples: totalSamples,
			Error:        err.Error(),
			CreatedAt:    session.StartedAt,
			UpdatedAt:    time.Now(),
		})
		return fmt.Errorf("stop capture: %w", err)
	}

	session.SetState(Transcribing)

	result, err := k.asr.Transcribe(ctx, chunks)
	if err != nil {
		session.SetState(Failed)
		_ = k.store.SaveRecovery(storage.SessionRecovery{
			ID:           session.ID,
			State:        "failed",
			PartialText:  session.GetPartial(),
			AudioSamples: totalSamples,
			Error:        err.Error(),
			CreatedAt:    session.StartedAt,
			UpdatedAt:    time.Now(),
		})
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
		_ = k.store.SaveRecovery(storage.SessionRecovery{
			ID:           session.ID,
			State:        "failed",
			PartialText:  session.GetPartial(),
			RawText:      session.RawText,
			Text:         session.FinalText,
			AudioSamples: totalSamples,
			Error:        fmt.Sprintf("paste: %v", err),
			CreatedAt:    session.StartedAt,
			UpdatedAt:    time.Now(),
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

	_ = k.store.SaveRecovery(storage.SessionRecovery{
		ID:           session.ID,
		State:        "completed",
		PartialText:  session.GetPartial(),
		RawText:      session.RawText,
		Text:         session.FinalText,
		AudioSamples: totalSamples,
		CreatedAt:    session.StartedAt,
		UpdatedAt:    time.Now(),
	})

	session.SetState(Complete)
	return nil
}
