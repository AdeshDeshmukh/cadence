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

// PrivacyPolicy defines data retention and logging rules for dictation sessions.
type PrivacyPolicy struct {
	SaveTranscripts bool // Persist successful transcripts to storage.
	SaveRecoveries  bool // Persist uncommitted/failed session recoveries.
	Incognito       bool // Complete ephemeral mode: zero storage persistence.
	RedactSensitive bool // Redact sensitive credentials/tokens in memory/logs.
}

// DefaultPrivacyPolicy provides default persistence behavior.
func DefaultPrivacyPolicy() PrivacyPolicy {
	return PrivacyPolicy{
		SaveTranscripts: true,
		SaveRecoveries:  true,
		Incognito:       false,
		RedactSensitive: true,
	}
}

type Kernel struct {
	capture audio.Capture
	asr     asr.Engine
	paste   paste.Paster
	process *postprocess.Pipeline
	store   storage.Store
	privacy PrivacyPolicy
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
		privacy: DefaultPrivacyPolicy(),
	}
}

// SetPrivacyPolicy updates the active privacy policy for the kernel.
func (k *Kernel) SetPrivacyPolicy(p PrivacyPolicy) {
	k.privacy = p
}

// PrivacyPolicy returns the current privacy policy of the kernel.
func (k *Kernel) PrivacyPolicy() PrivacyPolicy {
	return k.privacy
}

func (k *Kernel) saveRecovery(rec storage.SessionRecovery) {
	if k.privacy.Incognito || !k.privacy.SaveRecoveries {
		return
	}
	_ = k.store.SaveRecovery(rec)
}

func (k *Kernel) saveTranscript(tr storage.Transcript) error {
	if k.privacy.Incognito || !k.privacy.SaveTranscripts {
		return nil
	}
	return k.store.SaveTranscript(tr)
}

func (k *Kernel) Run(
	ctx context.Context,
	session *Session,
	pc postprocess.Context,
) error {
	session.SetState(Recording)
	k.saveRecovery(storage.SessionRecovery{
		ID:        session.ID,
		State:     "uncommitted",
		CreatedAt: session.StartedAt,
		UpdatedAt: time.Now(),
	})

	chunksCh, err := k.capture.Start()
	if err != nil {
		session.SetState(Failed)
		k.saveRecovery(storage.SessionRecovery{
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
	defer func() {
		// Memory hygiene: securely scrub raw audio samples from memory upon completion/exit
		for i := range chunks {
			for j := range chunks[i].Samples {
				chunks[i].Samples[j] = 0
			}
		}
	}()

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
			k.saveRecovery(storage.SessionRecovery{
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

	if app := session.GetApplication(); app != "" && pc.Application == "" {
		pc.Application = app
	}

	pc.OnPolish = func(orig, pol string) {
		session.SetUnpolishedText(orig)
	}

	if k.process != nil {
		session.FinalText = k.process.Apply(result.Text, pc)
	} else {
		session.FinalText = result.Text
	}
	if session.GetUnpolishedText() == "" {
		session.SetUnpolishedText(session.FinalText)
	}
	session.SetState(Pasting)

	if _, err := k.paste.Paste(session.FinalText); err != nil {
		_ = k.saveTranscript(storage.Transcript{
			ID: session.ID, RawText: session.RawText,
			Text: session.FinalText, Language: result.Language,
			Mode: pc.Mode, Application: pc.Application,
			CreatedAt: time.Now(),
		})
		k.saveRecovery(storage.SessionRecovery{
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

	if err := k.saveTranscript(storage.Transcript{
		ID: session.ID, RawText: session.RawText,
		Text: session.FinalText, Language: result.Language,
		Mode: pc.Mode, Application: pc.Application,
		CreatedAt: time.Now(),
	}); err != nil {
		session.SetState(Failed)
		return fmt.Errorf("store: %w", err)
	}

	k.saveRecovery(storage.SessionRecovery{
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
