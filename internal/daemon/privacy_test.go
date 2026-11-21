package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/config"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestApp_IPCPrivacyStatusAndSet(t *testing.T) {
	cfg := config.Default()
	store := storage.NewMemory()

	app := NewApp(
		cfg,
		WithStore(store),
	)

	ctx := context.Background()

	// 1. Get initial privacy_status
	resp, err := app.Handle(ctx, ipc.Request{Method: "privacy_status"})
	if err != nil {
		t.Fatalf("call privacy_status failed: %v", err)
	}
	statusMap, ok := resp.(map[string]any)
	if !ok || statusMap["ok"] != true {
		t.Fatalf("unexpected privacy_status response: %v", resp)
	}
	if statusMap["network_mode"] != "local_only" {
		t.Errorf("expected local_only network mode, got: %v", statusMap["network_mode"])
	}

	// 2. Set privacy options: disable transcripts, enable incognito
	setResp, err := app.Handle(ctx, ipc.Request{
		Method: "privacy_set",
		Params: map[string]any{
			"save_transcripts": false,
			"incognito":        true,
			"telemetry":        false,
			"redact_sensitive": true,
		},
	})
	if err != nil {
		t.Fatalf("call privacy_set failed: %v", err)
	}
	setMap, ok := setResp.(map[string]any)
	if !ok || setMap["save_transcripts"] != false || setMap["incognito"] != true {
		t.Fatalf("unexpected privacy_set response: %v", setResp)
	}

	// 3. Verify privacy_status matches updated configuration
	verifyResp, err := app.Handle(ctx, ipc.Request{Method: "privacy_status"})
	if err != nil {
		t.Fatalf("call privacy_status after update failed: %v", err)
	}
	vMap := verifyResp.(map[string]any)
	if vMap["save_transcripts"] != false || vMap["incognito"] != true {
		t.Fatalf("expected updated settings in privacy_status, got: %v", vMap)
	}
}

func TestApp_IPCPrivacyPurgeAndAudit(t *testing.T) {
	cfg := config.Default()
	store := storage.NewMemory()

	// Pre-populate with records
	_ = store.SaveTranscript(storage.Transcript{
		ID:        "tr-priv-1",
		RawText:   "hello",
		Text:      "Hello.",
		CreatedAt: time.Now(),
	})
	_ = store.SaveRecovery(storage.SessionRecovery{
		ID:        "rec-priv-1",
		State:     "completed",
		Text:      "Hello.",
		CreatedAt: time.Now(),
	})

	app := NewApp(
		cfg,
		WithStore(store),
	)

	ctx := context.Background()

	// 1. Audit before purge (with save_transcripts=false, should fail audit)
	_, _ = app.Handle(ctx, ipc.Request{
		Method: "privacy_set",
		Params: map[string]any{
			"save_transcripts": false,
		},
	})

	auditResp, err := app.Handle(ctx, ipc.Request{Method: "privacy_audit"})
	if err != nil {
		t.Fatalf("call privacy_audit failed: %v", err)
	}
	auditMap := auditResp.(map[string]any)
	if auditMap["compliant"] != false {
		t.Errorf("expected audit non-compliant when save_transcripts=false and transcripts exist, got: %v", auditMap)
	}

	// 2. Call privacy_purge
	purgeResp, err := app.Handle(ctx, ipc.Request{Method: "privacy_purge"})
	if err != nil {
		t.Fatalf("call privacy_purge failed: %v", err)
	}
	pMap := purgeResp.(map[string]any)
	if pMap["ok"] != true {
		t.Fatalf("unexpected purge response: %v", purgeResp)
	}

	// Verify store is completely empty
	transcripts, _ := store.ListTranscripts(10)
	if len(transcripts) != 0 {
		t.Fatalf("expected 0 transcripts after purge, found: %d", len(transcripts))
	}
	recoveries, _ := store.ListRecoveries(10)
	if len(recoveries) != 0 {
		t.Fatalf("expected 0 recoveries after purge, found: %d", len(recoveries))
	}

	// 3. Re-audit (should now be compliant)
	auditAfter, err := app.Handle(ctx, ipc.Request{Method: "privacy_audit"})
	if err != nil {
		t.Fatalf("call privacy_audit after purge failed: %v", err)
	}
	auditAfterMap := auditAfter.(map[string]any)
	if auditAfterMap["compliant"] != true {
		t.Fatalf("expected compliant audit after purge, got: %v", auditAfterMap)
	}
}
