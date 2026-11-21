package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

type privacyMockCapture struct {
	chunk audio.Chunk
}

func (p *privacyMockCapture) Devices() ([]audio.Device, error) { return nil, nil }
func (p *privacyMockCapture) Start() (<-chan audio.Chunk, error) {
	ch := make(chan audio.Chunk, 1)
	ch <- p.chunk
	close(ch)
	return ch, nil
}
func (p *privacyMockCapture) Stop() error { return nil }

type privacyMockPaster struct {
	pasted string
}

func (p *privacyMockPaster) Paste(text string) (paste.Result, error) {
	p.pasted = text
	return paste.Result{Method: "mock", Success: true}, nil
}

func TestPipeline_StrictPrivacyOptOut(t *testing.T) {
	samples := []float32{0.25, -0.5, 0.75, -0.1}
	capture := &privacyMockCapture{
		chunk: audio.Chunk{
			Samples:    samples,
			SampleRate: 16000,
			Timestamp:  time.Now(),
		},
	}
	engine := asr.MockEngine{Text: "Confidential financial figures."}
	paster := &privacyMockPaster{}
	store := storage.NewMemory()

	kernel := NewKernel(capture, engine, paster, postprocess.New(), store)
	// Explicitly opt OUT of saving transcripts
	kernel.SetPrivacyPolicy(PrivacyPolicy{
		SaveTranscripts: false,
		SaveRecoveries:  true,
		Incognito:       false,
		RedactSensitive: true,
	})

	session := NewSession("priv-test-1")
	pc := postprocess.Context{Mode: "normal", Application: "Slack"}

	err := kernel.Run(context.Background(), session, pc)
	if err != nil {
		t.Fatalf("kernel.Run failed: %v", err)
	}

	if paster.pasted != "Confidential financial figures." {
		t.Fatalf("expected pasted text, got: %q", paster.pasted)
	}

	// Verify ZERO transcripts saved to storage
	transcripts, err := store.ListTranscripts(10)
	if err != nil {
		t.Fatalf("ListTranscripts failed: %v", err)
	}
	if len(transcripts) != 0 {
		t.Fatalf("PRIVACY VIOLATION: expected 0 saved transcripts when SaveTranscripts=false, found %d: %v", len(transcripts), transcripts)
	}
}

func TestPipeline_IncognitoMode(t *testing.T) {
	samples := []float32{0.1, -0.2, 0.3}
	capture := &privacyMockCapture{
		chunk: audio.Chunk{
			Samples:    samples,
			SampleRate: 16000,
			Timestamp:  time.Now(),
		},
	}
	engine := asr.MockEngine{Text: "Top secret meeting notes."}
	paster := &privacyMockPaster{}
	store := storage.NewMemory()

	kernel := NewKernel(capture, engine, paster, postprocess.New(), store)
	// Incognito ephemeral mode
	kernel.SetPrivacyPolicy(PrivacyPolicy{
		SaveTranscripts: true,
		SaveRecoveries:  true,
		Incognito:       true, // Should completely override and suppress all disk/db writes
		RedactSensitive: true,
	})

	session := NewSession("priv-incognito-1")
	pc := postprocess.Context{Mode: "normal", Application: "Signal"}

	err := kernel.Run(context.Background(), session, pc)
	if err != nil {
		t.Fatalf("kernel.Run failed: %v", err)
	}

	// Verify ZERO transcripts
	transcripts, err := store.ListTranscripts(10)
	if err != nil {
		t.Fatalf("ListTranscripts failed: %v", err)
	}
	if len(transcripts) != 0 {
		t.Fatalf("INCOGNITO VIOLATION: expected 0 transcripts, found %d", len(transcripts))
	}

	// Verify ZERO session recoveries
	recoveries, err := store.ListRecoveries(10)
	if err != nil {
		t.Fatalf("ListRecoveries failed: %v", err)
	}
	if len(recoveries) != 0 {
		t.Fatalf("INCOGNITO VIOLATION: expected 0 recoveries in incognito mode, found %d", len(recoveries))
	}
}

func TestPipeline_AudioBufferZeroing(t *testing.T) {
	rawSamples := []float32{0.42, -0.88, 0.99, -0.33}
	capture := &privacyMockCapture{
		chunk: audio.Chunk{
			Samples:    rawSamples,
			SampleRate: 16000,
			Timestamp:  time.Now(),
		},
	}
	engine := asr.MockEngine{Text: "Audio buffer wiping test"}
	paster := &privacyMockPaster{}
	store := storage.NewMemory()

	kernel := NewKernel(capture, engine, paster, postprocess.New(), store)
	session := NewSession("priv-zero-1")
	pc := postprocess.Context{}

	err := kernel.Run(context.Background(), session, pc)
	if err != nil {
		t.Fatalf("kernel.Run failed: %v", err)
	}

	// Verify audio samples in RAM were wiped (zeroed out)
	for i, sample := range rawSamples {
		if sample != 0.0 {
			t.Errorf("MEMORY LEAK: sample[%d] was not zeroed out, value = %f", i, sample)
		}
	}
}

func TestPipeline_OptInTranscripts(t *testing.T) {
	capture := &privacyMockCapture{
		chunk: audio.Chunk{
			Samples:    []float32{0.1, -0.1},
			SampleRate: 16000,
			Timestamp:  time.Now(),
		},
	}
	engine := asr.MockEngine{Text: "Opted-in transcript."}
	paster := &privacyMockPaster{}
	store := storage.NewMemory()

	kernel := NewKernel(capture, engine, paster, postprocess.New(), store)
	kernel.SetPrivacyPolicy(PrivacyPolicy{
		SaveTranscripts: true,
		SaveRecoveries:  true,
		Incognito:       false,
		RedactSensitive: true,
	})

	session := NewSession("priv-optin-1")
	pc := postprocess.Context{Mode: "normal", Application: "Notes"}

	err := kernel.Run(context.Background(), session, pc)
	if err != nil {
		t.Fatalf("kernel.Run failed: %v", err)
	}

	transcripts, err := store.ListTranscripts(10)
	if err != nil {
		t.Fatalf("ListTranscripts failed: %v", err)
	}
	if len(transcripts) != 1 {
		t.Fatalf("expected 1 transcript, got %d", len(transcripts))
	}
	if transcripts[0].Text != "Opted-in transcript." {
		t.Errorf("expected text %q, got %q", "Opted-in transcript.", transcripts[0].Text)
	}
}
