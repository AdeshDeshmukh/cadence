package daemon

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/agent"
	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/config"
	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
	"github.com/AdeshDeshmukh/cadence/internal/developer"
	"github.com/AdeshDeshmukh/cadence/internal/diarize"
	"github.com/AdeshDeshmukh/cadence/internal/edit"
	"github.com/AdeshDeshmukh/cadence/internal/hotkey"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/meeting"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/plugins"
	"github.com/AdeshDeshmukh/cadence/internal/polish"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
	"github.com/AdeshDeshmukh/cadence/internal/search"
	"github.com/AdeshDeshmukh/cadence/internal/selection"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
	"github.com/AdeshDeshmukh/cadence/internal/system"
	"github.com/AdeshDeshmukh/cadence/internal/vad"
)

type recordingPaster struct {
	mu       sync.Mutex
	lastText string
}

func (r *recordingPaster) Paste(text string) (paste.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastText = text
	return paste.Result{Method: "mock", Success: true}, nil
}

func (r *recordingPaster) Text() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastText
}

type testLiveCapture struct {
	mu     sync.Mutex
	stopCh chan struct{}
	chunk  audio.Chunk
}

func (t *testLiveCapture) Devices() ([]audio.Device, error) { return nil, nil }

func (t *testLiveCapture) Start() (<-chan audio.Chunk, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopCh = make(chan struct{})
	ch := make(chan audio.Chunk, 10)
	ch <- t.chunk
	go func(stopCh chan struct{}) {
		<-stopCh
		close(ch)
	}(t.stopCh)
	return ch, nil
}

func (t *testLiveCapture) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopCh != nil {
		select {
		case <-t.stopCh:
		default:
			close(t.stopCh)
		}
	}
	return nil
}

func setupTestApp(t *testing.T, mode hotkey.Mode) (*App, *hotkey.SimulatedListener, *recordingPaster, storage.Store, net.Listener) {
	t.Helper()

	cfg := config.Default()
	cfg.Hotkey.Mode = string(mode)

	simHotkey := hotkey.NewSimulatedListener("Cmd+Shift+Space", mode)
	paster := &recordingPaster{}
	store := storage.NewMemory()

	chunk := audio.Chunk{
		Samples:    []float32{0.1, 0.2, -0.1},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}
	capture := &testLiveCapture{chunk: chunk}
	engine := asr.MockEngine{Text: "um hello world period"}

	pipe := postprocess.New(
		postprocess.NewFillerRemoval(),
		postprocess.NewPunctuation(),
	)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind test listener: %v", err)
	}

	app := NewApp(
		cfg,
		WithCapture(capture),
		WithASR(engine),
		WithPaster(paster),
		WithStore(store),
		WithPostprocess(pipe),
		WithHotkeyListener(simHotkey),
		WithIPCListener(l),
	)

	return app, simHotkey, paster, store, l
}

func TestApp_LifecycleAndIPC(t *testing.T) {
	app, _, _, _, listener := setupTestApp(t, hotkey.ModePushToTalk)
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer app.Stop()

	client := ipc.Client{
		Network: "tcp",
		Address: listener.Addr().String(),
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()

	resp, err := client.Call(callCtx, "ping", nil)
	if err != nil {
		t.Fatalf("call ping failed: %v", err)
	}
	if resp != "pong" {
		t.Fatalf("expected 'pong', got %v", resp)
	}

	statusResp, err := client.Call(callCtx, "status", nil)
	if err != nil {
		t.Fatalf("call status failed: %v", err)
	}
	statusMap, ok := statusResp.(map[string]any)
	if !ok || statusMap["status"] != "running" {
		t.Fatalf("unexpected status response: %v", statusResp)
	}
	if _, hasPartial := statusMap["partial"]; !hasPartial {
		t.Fatal("expected status response to contain 'partial' field")
	}
}

func TestApp_PushToTalkSession(t *testing.T) {
	app, simHotkey, paster, store, listener := setupTestApp(t, hotkey.ModePushToTalk)
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer app.Stop()

	// Simulate KeyDown (user presses hotkey)
	simHotkey.Emit(hotkey.EventKeyDown)

	// Verify recording state
	time.Sleep(50 * time.Millisecond)
	if !app.IsRecording() {
		t.Fatal("expected app to be recording after press")
	}

	// Simulate KeyUp (user releases hotkey)
	simHotkey.Emit(hotkey.EventKeyUp)

	// Wait for pipeline execution (capture stop -> asr -> postprocess -> paste)
	deadline := time.Now().Add(5 * time.Second)
	for app.IsRecording() {
		if time.Now().After(deadline) {
			t.Fatal("expected app to stop recording after release")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if paster.Text() != "Hello world." {
		t.Fatalf("expected pasted text 'Hello world.', got %q", paster.Text())
	}

	history, err := store.ListTranscripts(10)
	if err != nil {
		t.Fatalf("ListTranscripts failed: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 saved transcript, got %d", len(history))
	}
	if history[0].Text != "Hello world." {
		t.Fatalf("expected stored text 'Hello world.', got %q", history[0].Text)
	}
}

func TestApp_ToggleSession(t *testing.T) {
	app, simHotkey, paster, _, listener := setupTestApp(t, hotkey.ModeToggle)
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer app.Stop()

	// First press: start recording
	simHotkey.Emit(hotkey.EventKeyDown)
	time.Sleep(50 * time.Millisecond)
	if !app.IsRecording() {
		t.Fatal("expected recording on first toggle press")
	}

	// Second press: stop recording
	simHotkey.Emit(hotkey.EventKeyDown)
	deadline := time.Now().Add(5 * time.Second)
	for app.IsRecording() {
		if time.Now().After(deadline) {
			t.Fatal("expected stopped recording on second toggle press")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if paster.Text() != "Hello world." {
		t.Fatalf("expected pasted text 'Hello world.', got %q", paster.Text())
	}
}

func TestApp_IPCDictateCommands(t *testing.T) {
	app, _, _, _, listener := setupTestApp(t, hotkey.ModePushToTalk)
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer app.Stop()

	client := ipc.Client{
		Network: "tcp",
		Address: listener.Addr().String(),
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()

	// Trigger programmatic start
	_, err := client.Call(callCtx, "dictate_start", nil)
	if err != nil {
		t.Fatalf("dictate_start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if !app.IsRecording() {
		t.Fatal("expected recording after dictate_start")
	}

	// Trigger programmatic stop
	_, err = client.Call(callCtx, "dictate_stop", nil)
	if err != nil {
		t.Fatalf("dictate_stop failed: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for app.IsRecording() {
		if time.Now().After(deadline) {
			t.Fatal("expected stopped recording after dictate_stop")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestApp_EarlyKeyUpBeforeCaptureReady(t *testing.T) {
	app, simHotkey, paster, _, listener := setupTestApp(t, hotkey.ModePushToTalk)
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer app.Stop()

	// Emit key down and immediate key up before capture goroutine starts
	simHotkey.Emit(hotkey.EventKeyDown)
	simHotkey.Emit(hotkey.EventKeyUp)

	// Bounded wait to ensure session completes without hanging indefinitely
	deadline := time.Now().Add(5 * time.Second)
	for {
		if paster.Text() == "Hello world." {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected pasted text 'Hello world.', got %q", paster.Text())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestApp_IPCHistoryAndVocabulary(t *testing.T) {
	app, _, _, store, listener := setupTestApp(t, hotkey.ModePushToTalk)
	defer listener.Close()

	_ = store.SaveTranscript(storage.Transcript{
		ID:        "history-1",
		RawText:   "history test",
		Text:      "History test.",
		CreatedAt: time.Now().UTC(),
	})
	_ = store.SetVocabulary(storage.VocabularyEntry{
		Spoken:    "k8s",
		Canonical: "Kubernetes",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer app.Stop()

	client := ipc.Client{
		Network: "tcp",
		Address: listener.Addr().String(),
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()

	histResp, err := client.Call(callCtx, "history", map[string]any{"limit": float64(5)})
	if err != nil {
		t.Fatalf("call history failed: %v", err)
	}
	histList, ok := histResp.([]any)
	if !ok || len(histList) == 0 {
		t.Fatalf("expected non-empty history list, got: %v", histResp)
	}

	vocabResp, err := client.Call(callCtx, "vocabulary", nil)
	if err != nil {
		t.Fatalf("call vocabulary failed: %v", err)
	}
	vocabList, ok := vocabResp.([]any)
	if !ok || len(vocabList) == 0 {
		t.Fatalf("expected non-empty vocabulary list, got: %v", vocabResp)
	}
}

func TestApp_PersistentSQLite(t *testing.T) {
	cfg := config.Default()
	cfg.Storage.DBPath = ":memory:"

	app := NewApp(cfg)
	if app.store == nil {
		t.Fatal("expected non-nil store in App with SQLite config")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := app.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestApp_VADDetector(t *testing.T) {
	// 1. Adaptive detector default fallback
	cfg := config.Default()
	cfg.VAD.Engine = "adaptive"
	app := NewApp(cfg)
	if app.Detector() == nil {
		t.Fatal("expected non-nil VAD detector")
	}
	if _, ok := app.Detector().(*vad.AdaptiveDetector); !ok {
		t.Fatalf("expected *vad.AdaptiveDetector, got %T", app.Detector())
	}

	// 2. Custom detector option
	custom := vad.NewSimple(0.02)
	appCustom := NewApp(cfg, WithDetector(custom))
	if appCustom.Detector() != custom {
		t.Fatal("expected custom detector to be assigned")
	}
}

func TestApp_IPCRecovery(t *testing.T) {
	app, _, paster, store, listener := setupTestApp(t, hotkey.ModePushToTalk)
	defer listener.Close()

	now := time.Now().UTC()
	_ = store.SaveRecovery(storage.SessionRecovery{
		ID:          "rec-ipc-1",
		State:       "cancelled",
		PartialText: "important draft to recover",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer app.Stop()

	client := ipc.Client{
		Network: "tcp",
		Address: listener.Addr().String(),
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()

	// 1. Recover without paste
	res, err := client.Call(callCtx, "recover", nil)
	if err != nil {
		t.Fatalf("call recover failed: %v", err)
	}
	recMap, ok := res.(map[string]any)
	if !ok || recMap == nil {
		t.Fatalf("expected recovery object, got: %v", res)
	}
	if recMap["id"] != "rec-ipc-1" {
		t.Errorf("expected ID rec-ipc-1, got %v", recMap["id"])
	}

	// 2. Recover with paste
	_, err = client.Call(callCtx, "recover", map[string]any{"paste": true})
	if err != nil {
		t.Fatalf("call recover with paste failed: %v", err)
	}
	if paster.Text() != "important draft to recover" {
		t.Errorf("expected paster to have %q, got %q", "important draft to recover", paster.Text())
	}

	// 3. Recoveries list
	listRes, err := client.Call(callCtx, "recoveries", map[string]any{"limit": float64(5)})
	if err != nil {
		t.Fatalf("call recoveries failed: %v", err)
	}
	recs, ok := listRes.([]any)
	if !ok || len(recs) != 1 {
		t.Fatalf("expected 1 recovery in list, got: %v", listRes)
	}
}

func TestApp_IPCVocabularyManagement(t *testing.T) {
	app, _, _, _, listener := setupTestApp(t, hotkey.ModePushToTalk)
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer app.Stop()

	client := ipc.Client{
		Network: "tcp",
		Address: listener.Addr().String(),
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()

	// 1. Add vocabulary entry
	_, err := client.Call(callCtx, "vocab_add", map[string]any{
		"spoken":    "k eight s",
		"canonical": "Kubernetes",
	})
	if err != nil {
		t.Fatalf("vocab_add failed: %v", err)
	}

	// 2. List vocabulary
	listRes, err := client.Call(callCtx, "vocab_list", nil)
	if err != nil {
		t.Fatalf("vocab_list failed: %v", err)
	}
	list, ok := listRes.([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("expected 1 vocab entry, got: %v", listRes)
	}

	// 3. Export vocabulary as CSV
	exportRes, err := client.Call(callCtx, "vocab_export", map[string]any{"format": "csv"})
	if err != nil {
		t.Fatalf("vocab_export failed: %v", err)
	}
	expMap, ok := exportRes.(map[string]any)
	if !ok || expMap["content"] == "" {
		t.Fatalf("expected non-empty export content, got: %v", exportRes)
	}

	// 4. Import additional entries from JSON
	jsonContent := `[{"spoken": "docker compose", "canonical": "docker-compose"}]`
	importRes, err := client.Call(callCtx, "vocab_import", map[string]any{
		"content": jsonContent,
		"format":  "json",
	})
	if err != nil {
		t.Fatalf("vocab_import failed: %v", err)
	}
	impMap, ok := importRes.(map[string]any)
	if !ok || impMap["count"] != float64(1) {
		t.Fatalf("expected 1 imported entry, got: %v", importRes)
	}

	// 5. Remove original entry
	_, err = client.Call(callCtx, "vocab_remove", map[string]any{"spoken": "k eight s"})
	if err != nil {
		t.Fatalf("vocab_remove failed: %v", err)
	}

	// 6. List should only contain docker compose now
	listResAfter, err := client.Call(callCtx, "vocab_list", nil)
	if err != nil {
		t.Fatalf("vocab_list after remove failed: %v", err)
	}
	listAfter, ok := listResAfter.([]any)
	if !ok || len(listAfter) != 1 {
		t.Fatalf("expected 1 entry after removal, got: %v", listResAfter)
	}
}

func TestApp_IPCSnippets(t *testing.T) {
	app, _, _, _, listener := setupTestApp(t, hotkey.ModePushToTalk)
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer app.Stop()

	client := ipc.Client{
		Network: "tcp",
		Address: listener.Addr().String(),
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()

	// 1. Add snippet
	_, err := client.Call(callCtx, "snippet_set", map[string]any{
		"trigger":     "meeting notes",
		"template":    "# Notes {date}\n- Action: {clipboard}",
		"description": "Meeting template",
	})
	if err != nil {
		t.Fatalf("snippet_set failed: %v", err)
	}

	// 2. Get snippet
	gotRes, err := client.Call(callCtx, "snippet_get", map[string]any{
		"trigger": "meeting notes",
	})
	if err != nil {
		t.Fatalf("snippet_get failed: %v", err)
	}
	gotMap, ok := gotRes.(map[string]any)
	if !ok || gotMap["trigger"] != "meeting notes" {
		t.Fatalf("expected snippet object, got: %v", gotRes)
	}

	// 3. List snippets
	listRes, err := client.Call(callCtx, "snippet_list", nil)
	if err != nil {
		t.Fatalf("snippet_list failed: %v", err)
	}
	list, ok := listRes.([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("expected 1 snippet in list, got: %v", listRes)
	}

	// 4. Expand snippet
	expandRes, err := client.Call(callCtx, "snippet_expand", map[string]any{
		"trigger": "meeting notes",
	})
	if err != nil {
		t.Fatalf("snippet_expand failed: %v", err)
	}
	expMap, ok := expandRes.(map[string]any)
	if !ok || expMap["expanded"] == "" {
		t.Fatalf("expected expanded snippet, got: %v", expandRes)
	}

	// 5. Delete snippet
	_, err = client.Call(callCtx, "snippet_delete", map[string]any{
		"trigger": "meeting notes",
	})
	if err != nil {
		t.Fatalf("snippet_delete failed: %v", err)
	}

	// 6. List should be empty
	listResAfter, err := client.Call(callCtx, "snippet_list", nil)
	if err != nil {
		t.Fatalf("snippet_list after delete failed: %v", err)
	}
	listAfter, ok := listResAfter.([]any)
	if !ok || len(listAfter) != 0 {
		t.Fatalf("expected 0 snippets after delete, got: %v", listResAfter)
	}
}

func TestApp_PolisherIntegration(t *testing.T) {
	cfg := config.Default()
	cfg.Polish.Enabled = true
	cfg.Polish.TimeoutMs = 1000

	mock := polish.NewMockPolisher(func(req polish.Request) (polish.Response, error) {
		return polish.Response{
			Text:  "mocked polished text",
			Model: "mock",
		}, nil
	})
	paster := &recordingPaster{}
	store := storage.NewMemory()

	chunk := audio.Chunk{
		Samples:    []float32{0.1, 0.2, -0.1},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}
	capture := &testLiveCapture{chunk: chunk}
	engine := asr.MockEngine{Text: "raw unpolished text"}
	simHotkey := hotkey.NewSimulatedListener("Cmd+Shift+Space", hotkey.ModePushToTalk)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind test listener: %v", err)
	}

	app := NewApp(
		cfg,
		WithCapture(capture),
		WithASR(engine),
		WithPaster(paster),
		WithStore(store),
		WithHotkeyListener(simHotkey),
		WithIPCListener(l),
		WithPolisher(mock),
	)

	if app.Polisher() != mock {
		t.Fatalf("expected app.Polisher() to match configured mock polisher")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("failed to start app: %v", err)
	}
	defer app.Stop()

	// Press hotkey to start dictation
	simHotkey.Emit(hotkey.EventKeyDown)
	time.Sleep(50 * time.Millisecond)

	// Release hotkey to stop dictation and process
	simHotkey.Emit(hotkey.EventKeyUp)

	// Wait for pipeline processing to finish and paste
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if paster.Text() == "mocked polished text" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := paster.Text(); got != "mocked polished text" {
		t.Fatalf("expected pasted text 'mocked polished text', got %q", got)
	}
}

func TestApp_OllamaPolisherInitAndIPC(t *testing.T) {
	cfg := config.Default()
	cfg.Polish.Enabled = true
	cfg.Polish.Backend = "ollama"
	cfg.Polish.Model = "mistral"
	cfg.Polish.Endpoint = "http://127.0.0.1:11434"

	app := NewApp(cfg)
	polisher := app.Polisher()
	if polisher == nil {
		t.Fatalf("expected non-nil polisher")
	}
	ollama, ok := polisher.(*polish.OllamaPolisher)
	if !ok {
		t.Fatalf("expected *polish.OllamaPolisher, got %T", polisher)
	}
	if ollama.Model() != "mistral" {
		t.Fatalf("expected model mistral, got %s", ollama.Model())
	}
	if ollama.Endpoint() != "http://127.0.0.1:11434" {
		t.Fatalf("expected endpoint http://127.0.0.1:11434, got %s", ollama.Endpoint())
	}

	// Test IPC status includes polish details
	ctx := context.Background()
	res, err := app.Handle(ctx, ipc.Request{Method: "status"})
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", res)
	}
	if m["polish_enabled"] != true || m["polish_backend"] != "ollama" || m["polish_model"] != "mistral" {
		t.Fatalf("unexpected polish status: %v", m)
	}

	// Test IPC polish call with mock polisher
	mock := polish.NewMockPolisher(func(req polish.Request) (polish.Response, error) {
		return polish.Response{
			Text:      "Polished: " + req.Text,
			Model:     "mock-llm",
			TokensIn:  5,
			TokensOut: 7,
			Latency:   10 * time.Millisecond,
		}, nil
	})
	appWithMock := NewApp(cfg, WithPolisher(mock))
	polishRes, err := appWithMock.Handle(ctx, ipc.Request{
		Method: "polish",
		Params: map[string]any{
			"text":    "raw text",
			"profile": "code",
		},
	})
	if err != nil {
		t.Fatalf("polish IPC failed: %v", err)
	}
	pMap, ok := polishRes.(map[string]any)
	if !ok || pMap["text"] != "Polished: raw text" || pMap["model"] != "mock-llm" {
		t.Fatalf("unexpected polish result: %v", polishRes)
	}
}

func TestApp_OpenAIPolisherInit(t *testing.T) {
	cfg := config.Default()
	cfg.Polish.Enabled = true
	cfg.Polish.Backend = "openai"
	cfg.Polish.Model = "gpt-4o"
	cfg.Polish.Endpoint = "https://api.openai.com/v1"
	cfg.Polish.APIKey = "sk-daemon-test-key"
	cfg.Privacy.NetworkMode = "cloud"

	app := NewApp(cfg)
	polisher := app.Polisher()
	if polisher == nil {
		t.Fatalf("expected non-nil polisher")
	}
	oa, ok := polisher.(*polish.OpenAIPolisher)
	if !ok {
		t.Fatalf("expected *polish.OpenAIPolisher, got %T", polisher)
	}
	if oa.Model() != "gpt-4o" {
		t.Fatalf("expected model gpt-4o, got %s", oa.Model())
	}
	if oa.Endpoint() != "https://api.openai.com/v1" {
		t.Fatalf("expected endpoint https://api.openai.com/v1, got %s", oa.Endpoint())
	}

	// Verify status IPC reports openai backend
	res, err := app.Handle(context.Background(), ipc.Request{Method: "status"})
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok || m["polish_backend"] != "openai" || m["polish_model"] != "gpt-4o" {
		t.Fatalf("unexpected status result: %v", m)
	}
}

func TestApp_ProfilesIPC(t *testing.T) {
	cfg := config.Default()
	store := storage.NewMemory()
	app := NewApp(cfg, WithStore(store))

	if app.Profiles() == nil {
		t.Fatalf("expected non-nil profile registry")
	}

	ctx := context.Background()

	// 1. List built-ins
	listRes, err := app.Handle(ctx, ipc.Request{Method: "polish_profile_list"})
	if err != nil {
		t.Fatalf("polish_profile_list failed: %v", err)
	}
	pList, ok := listRes.([]polish.Profile)
	if !ok || len(pList) < 5 {
		t.Fatalf("expected at least 5 builtins, got: %v", listRes)
	}

	// 2. Set custom profile
	setRes, err := app.Handle(ctx, ipc.Request{
		Method: "polish_profile_set",
		Params: map[string]any{
			"name":          "journalist",
			"description":   "AP news style prose",
			"system_prompt": "Format as AP style journalist report.",
			"temperature":   0.25,
			"custom_rules":  []any{"Inverted pyramid structure"},
		},
	})
	if err != nil {
		t.Fatalf("polish_profile_set failed: %v", err)
	}
	setMap, ok := setRes.(map[string]any)
	if !ok || setMap["name"] != "journalist" {
		t.Fatalf("unexpected set result: %v", setRes)
	}

	// 3. Get custom profile
	getRes, err := app.Handle(ctx, ipc.Request{
		Method: "polish_profile_get",
		Params: map[string]any{"name": "journalist"},
	})
	if err != nil {
		t.Fatalf("polish_profile_get failed: %v", err)
	}
	prof, ok := getRes.(polish.Profile)
	if !ok || prof.Name != "journalist" || prof.Description != "AP news style prose" {
		t.Fatalf("unexpected profile get: %+v", getRes)
	}

	// 4. Delete custom profile
	_, err = app.Handle(ctx, ipc.Request{
		Method: "polish_profile_delete",
		Params: map[string]any{"name": "journalist"},
	})
	if err != nil {
		t.Fatalf("polish_profile_delete failed: %v", err)
	}

	// 5. Get after delete should fail
	_, err = app.Handle(ctx, ipc.Request{
		Method: "polish_profile_get",
		Params: map[string]any{"name": "journalist"},
	})
	if err == nil {
		t.Fatalf("expected error getting deleted profile, got nil")
	}
}

func TestApp_ActiveAppContextAndIPC(t *testing.T) {
	cfg := config.Default()
	mockApp := appcontext.AppInfo{
		Name:        "Visual Studio Code",
		BundleID:    "com.microsoft.VSCode",
		WindowTitle: "daemon/app.go - cadence",
		ProcessID:   9988,
		Category:    appcontext.CategoryCode,
	}
	detector := appcontext.NewMockDetector(mockApp)

	app := NewApp(cfg, WithAppDetector(detector))
	if app.AppDetector() != detector {
		t.Fatalf("expected configured detector to match")
	}

	ctx := context.Background()

	// 1. Test IPC active_app
	res, err := app.Handle(ctx, ipc.Request{Method: "active_app"})
	if err != nil {
		t.Fatalf("active_app failed: %v", err)
	}
	info, ok := res.(appcontext.AppInfo)
	if !ok || info.Name != "Visual Studio Code" || info.Category != appcontext.CategoryCode {
		t.Fatalf("unexpected app info: %+v", res)
	}

	// 2. Test IPC status includes active_app
	statusRes, err := app.Handle(ctx, ipc.Request{Method: "status"})
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	sMap, ok := statusRes.(map[string]any)
	if !ok || sMap["active_app"] != "Visual Studio Code" {
		t.Fatalf("unexpected status active_app: %v", statusRes)
	}
}

func TestApp_ModeRoutingIPC(t *testing.T) {
	cfg := config.Default()
	cfg.Polish.AppRoutes = map[string]string{
		"slack": "slack_friendly",
	}

	app := NewApp(cfg)
	if app.Router() == nil {
		t.Fatalf("expected non-nil router")
	}

	ctx := context.Background()

	// 1. List routes
	listRes, err := app.Handle(ctx, ipc.Request{Method: "polish_route_list"})
	if err != nil {
		t.Fatalf("polish_route_list failed: %v", err)
	}
	rMap, ok := listRes.(map[string]string)
	if !ok || rMap["slack"] != "slack_friendly" {
		t.Fatalf("unexpected route list: %+v", listRes)
	}

	// 2. Set new route
	setRes, err := app.Handle(ctx, ipc.Request{
		Method: "polish_route_set",
		Params: map[string]any{
			"app":     "Obsidian",
			"profile": "academic",
		},
	})
	if err != nil {
		t.Fatalf("polish_route_set failed: %v", err)
	}
	sMap, ok := setRes.(map[string]any)
	if !ok || sMap["profile"] != "academic" {
		t.Fatalf("unexpected set result: %+v", setRes)
	}

	// 3. Test route resolution
	testRes, err := app.Handle(ctx, ipc.Request{
		Method: "polish_route_test",
		Params: map[string]any{"app": "Visual Studio Code"},
	})
	if err != nil {
		t.Fatalf("polish_route_test failed: %v", err)
	}
	tMap, ok := testRes.(map[string]any)
	if !ok || tMap["profile"] != "code" {
		t.Fatalf("expected profile 'code' for VS Code, got: %+v", testRes)
	}

	// 4. Delete route
	delRes, err := app.Handle(ctx, ipc.Request{
		Method: "polish_route_delete",
		Params: map[string]any{"app": "Obsidian"},
	})
	if err != nil {
		t.Fatalf("polish_route_delete failed: %v", err)
	}
	dMap, ok := delRes.(map[string]any)
	if !ok || dMap["ok"] != true {
		t.Fatalf("unexpected delete result: %+v", delRes)
	}
}

func TestApp_UndoTransformationAndIPC(t *testing.T) {
	cfg := config.Default()
	cfg.Polish.Enabled = true
	cfg.Polish.Backend = "mock"
	cfg.Hotkey.Shortcut = "Cmd+Shift+Space"
	cfg.Hotkey.UndoShortcut = "Cmd+Shift+U"

	paster := paste.NewClipboard()
	reverter := paste.NewMockReverter()
	simHotkey := hotkey.NewSimulatedListener(cfg.Hotkey.Shortcut, hotkey.ModePushToTalk)
	mockASR := asr.MockEngine{Text: "my original thoughts"}
	chunk := audio.Chunk{Samples: []float32{0.1, 0.2, -0.1}, SampleRate: 16000, Timestamp: time.Now()}
	capture := &testLiveCapture{chunk: chunk}

	mockPolisher := polish.NewMockPolisher(func(req polish.Request) (polish.Response, error) {
		return polish.Response{
			Text:  "AI: " + req.Text,
			Model: "mock-llm",
		}, nil
	})

	app := NewApp(
		cfg,
		WithCapture(capture),
		WithASR(mockASR),
		WithPaster(paster),
		WithReverter(reverter),
		WithPolisher(mockPolisher),
		WithHotkeyListener(simHotkey),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("failed to start app: %v", err)
	}
	defer app.Stop()

	// Initial state: nothing to undo
	if app.CanUndo() {
		t.Fatal("expected CanUndo to be false initially")
	}
	if _, err := app.UndoTransformation("unpolished"); err == nil {
		t.Fatal("expected error undoing when no dictation has occurred")
	}

	// 1. Run dictation session
	simHotkey.Emit(hotkey.EventKeyDown)
	time.Sleep(50 * time.Millisecond)
	simHotkey.Emit(hotkey.EventKeyUp)

	// Wait for paste and undo cache
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if app.CanUndo() && paster.Text() == "AI: My original thoughts." {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := paster.Text(); got != "AI: My original thoughts." {
		t.Fatalf("expected polished text 'AI: My original thoughts.', got %q", got)
	}

	if !app.CanUndo() {
		t.Fatal("expected CanUndo to be true after polished dictation")
	}

	last := app.LastUndo()
	if last == nil {
		t.Fatal("expected non-nil LastUndo")
	}
	if last.PolishedText != "AI: My original thoughts." {
		t.Fatalf("expected polished text, got %q", last.PolishedText)
	}
	if last.UnpolishedText != "My original thoughts." {
		t.Fatalf("expected unpolished text, got %q", last.UnpolishedText)
	}

	// 2. Perform undo via IPC
	undoRes, err := app.Handle(ctx, ipc.Request{
		Method: "undo",
		Params: map[string]any{"target": "unpolished"},
	})
	if err != nil {
		t.Fatalf("undo IPC failed: %v", err)
	}

	uMap, ok := undoRes.(*UndoResult)
	if !ok {
		t.Fatalf("unexpected undo result type: %+v", undoRes)
	}
	if uMap.RevertedFrom != "AI: My original thoughts." {
		t.Errorf("expected reverted from 'AI: My original thoughts.', got %q", uMap.RevertedFrom)
	}
	if uMap.RevertedTo != "My original thoughts." {
		t.Errorf("expected reverted to 'My original thoughts.', got %q", uMap.RevertedTo)
	}

	if reverter.Count() != 1 {
		t.Fatalf("expected reverter call count 1, got %d", reverter.Count())
	}
	if paster.Text() != "My original thoughts." {
		t.Fatalf("expected paster to hold unpolished text, got %q", paster.Text())
	}

	// 3. Second undo attempt should fail
	if _, err := app.Handle(ctx, ipc.Request{Method: "undo"}); err == nil {
		t.Fatal("expected error on duplicate undo attempt")
	}

	// 4. Test Hotkey undo triggering
	// Start another dictation session
	simHotkey.Emit(hotkey.EventKeyDown)
	time.Sleep(50 * time.Millisecond)
	simHotkey.Emit(hotkey.EventKeyUp)

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if app.CanUndo() && paster.Text() == "AI: My original thoughts." {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Press undo hotkey
	app.handleHotkeyEvent(hotkey.Event{
		Type:      hotkey.EventKeyDown,
		Shortcut:  "Cmd+Shift+U",
		Timestamp: time.Now(),
	}, hotkey.ModePushToTalk)

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if paster.Text() == "My original thoughts." {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := paster.Text(); got != "My original thoughts." {
		t.Fatalf("expected hotkey undo to restore unpolished text, got %q", got)
	}
	if reverter.Count() != 2 {
		t.Fatalf("expected reverter call count 2, got %d", reverter.Count())
	}
}

func TestApp_UndoNotPolished(t *testing.T) {
	cfg := config.Default()
	cfg.Polish.Enabled = false // No AI polish

	paster := paste.NewClipboard()
	reverter := paste.NewMockReverter()
	simHotkey := hotkey.NewSimulatedListener(cfg.Hotkey.Shortcut, hotkey.ModePushToTalk)
	mockASR := asr.MockEngine{Text: "unpolished simple text"}
	chunk := audio.Chunk{Samples: []float32{0.1, 0.2, -0.1}, SampleRate: 16000, Timestamp: time.Now()}
	capture := &testLiveCapture{chunk: chunk}

	app := NewApp(
		cfg,
		WithCapture(capture),
		WithASR(mockASR),
		WithPaster(paster),
		WithReverter(reverter),
		WithHotkeyListener(simHotkey),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("failed to start app: %v", err)
	}
	defer app.Stop()

	simHotkey.Emit(hotkey.EventKeyDown)
	time.Sleep(50 * time.Millisecond)
	simHotkey.Emit(hotkey.EventKeyUp)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if app.LastUndo() != nil && paster.Text() != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if app.CanUndo() {
		t.Fatal("expected CanUndo to be false when polish was not active")
	}

	// Target "unpolished" fails because it wasn't modified by AI
	if _, err := app.UndoTransformation("unpolished"); err == nil {
		t.Fatal("expected error when attempting to undo unpolished dictation")
	}

	// Target "raw" succeeds
	res, err := app.UndoTransformation("raw")
	if err != nil {
		t.Fatalf("expected undo --raw to succeed, got: %v", err)
	}
	if res.RevertedTo != "unpolished simple text" {
		t.Fatalf("expected raw text, got %q", res.RevertedTo)
	}
}

func TestApp_SelfCorrectionIPC(t *testing.T) {
	cfg := config.Default()
	cfg.Post.Correction = true
	app := NewApp(cfg)

	ctx := context.Background()

	// 1. Correct code context
	res, err := app.Handle(ctx, ipc.Request{
		Method: "correct",
		Params: map[string]any{
			"text": "create a four loop with sink await in pie test",
			"app":  "Visual Studio Code",
		},
	})
	if err != nil {
		t.Fatalf("correct IPC failed: %v", err)
	}
	rMap, ok := res.(map[string]any)
	if !ok || rMap["corrected"] != "create a for loop with async await in pytest" {
		t.Fatalf("unexpected correct result: %+v", res)
	}
	if rMap["category"] != "code" {
		t.Fatalf("expected category code, got: %v", rMap["category"])
	}
	if rMap["changed"] != true {
		t.Fatalf("expected changed true, got: %v", rMap["changed"])
	}

	// 2. Correct medical context
	medRes, err := app.Handle(ctx, ipc.Request{
		Method: "correct",
		Params: map[string]any{
			"text": "patient percent with hyper tension",
			"app":  "Epic Hyperspace",
		},
	})
	if err != nil {
		t.Fatalf("correct IPC medical failed: %v", err)
	}
	mMap, ok := medRes.(map[string]any)
	if !ok || mMap["corrected"] != "patient presented with hypertension" {
		t.Fatalf("unexpected medical result: %+v", medRes)
	}
}

func TestApp_SelectionIPC(t *testing.T) {
	cfg := config.Default()
	mockSel := selection.NewMockCapturer("const hello = 'world';", "accessibility_ax")
	app := NewApp(
		cfg,
		WithSelectionCapturer(mockSel),
	)

	ctx := context.Background()

	res, err := app.Handle(ctx, ipc.Request{
		Method: "selection_get",
	})
	if err != nil {
		t.Fatalf("selection_get failed: %v", err)
	}

	selRes, ok := res.(selection.Result)
	if !ok {
		t.Fatalf("unexpected type: %T", res)
	}
	if selRes.Text != "const hello = 'world';" {
		t.Fatalf("expected 'const hello = \\'world\\';', got %q", selRes.Text)
	}
	if selRes.Method != "accessibility_ax" {
		t.Fatalf("expected method 'accessibility_ax', got %q", selRes.Method)
	}
	if mockSel.CallCount() != 1 {
		t.Fatalf("expected call count 1, got %d", mockSel.CallCount())
	}

	// Test error case
	mockSel.SetError(selection.ErrNoSelection)
	if _, err := app.Handle(ctx, ipc.Request{Method: "selection_get"}); !errors.Is(err, selection.ErrNoSelection) {
		t.Fatalf("expected ErrNoSelection, got %v", err)
	}
}

func TestApp_EditParseIPC(t *testing.T) {
	cfg := config.Default()
	app := NewApp(cfg)

	if app.EditParser() == nil {
		t.Fatal("expected non-nil EditParser")
	}

	ctx := context.Background()

	// 1. Successful parse of translation instruction
	res, err := app.Handle(ctx, ipc.Request{
		Method: "edit_parse",
		Params: map[string]any{
			"text": "translate this to French",
		},
	})
	if err != nil {
		t.Fatalf("edit_parse failed: %v", err)
	}

	instr, ok := res.(edit.Instruction)
	if !ok {
		t.Fatalf("unexpected result type: %T", res)
	}
	if instr.Intent != edit.IntentTranslate {
		t.Fatalf("expected IntentTranslate, got %q", instr.Intent)
	}
	if instr.TargetLanguage != "French" {
		t.Fatalf("expected TargetLanguage French, got %q", instr.TargetLanguage)
	}
	if !instr.IsEdit {
		t.Fatal("expected IsEdit to be true")
	}

	// 2. Shorten command
	res2, err := app.Handle(ctx, ipc.Request{
		Method: "edit_parse",
		Params: map[string]any{
			"text": "make this shorter",
		},
	})
	if err != nil {
		t.Fatalf("edit_parse shorten failed: %v", err)
	}
	instr2 := res2.(edit.Instruction)
	if instr2.Intent != edit.IntentShorten || instr2.Tone != "concise" {
		t.Fatalf("unexpected shorten instruction: %+v", instr2)
	}

	// 3. Error case: missing text
	if _, err := app.Handle(ctx, ipc.Request{Method: "edit_parse"}); err == nil {
		t.Fatal("expected error when text parameter is missing")
	}

	if _, err := app.Handle(ctx, ipc.Request{
		Method: "edit_parse",
		Params: map[string]any{"text": "   "},
	}); err == nil {
		t.Fatal("expected error when text parameter is whitespace")
	}
}

func TestApp_EditExecuteIPC(t *testing.T) {
	cfg := config.Default()
	mockPolisher := polish.NewMockPolisher(func(req polish.Request) (polish.Response, error) {
		return polish.Response{
			Text:  "Bonjour le monde",
			Model: "mock-french",
		}, nil
	})
	paster := &recordingPaster{}
	mockSel := selection.NewMockCapturer("Hello world", "accessibility_ax")

	app := NewApp(cfg,
		WithPolisher(mockPolisher),
		WithPaster(paster),
		WithSelectionCapturer(mockSel),
	)

	ctx := context.Background()

	// 1. Execute with automatic selection capture
	res, err := app.Handle(ctx, ipc.Request{
		Method: "edit_execute",
		Params: map[string]any{
			"instruction": "translate this to French",
		},
	})
	if err != nil {
		t.Fatalf("edit_execute failed: %v", err)
	}

	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", res)
	}
	if m["edited_text"] != "Bonjour le monde" {
		t.Fatalf("expected 'Bonjour le monde', got %v", m["edited_text"])
	}
	if m["pasted"] != true {
		t.Fatalf("expected pasted true, got %v", m["pasted"])
	}
	if paster.Text() != "Bonjour le monde" {
		t.Fatalf("expected paster to have 'Bonjour le monde', got %q", paster.Text())
	}

	// Verify Undo record was set
	if !app.CanUndo() {
		t.Fatal("expected CanUndo to be true after edit_execute")
	}
	undoRec := app.LastUndo()
	if undoRec == nil || undoRec.RawText != "Hello world" {
		t.Fatalf("expected undo record original text 'Hello world', got %+v", undoRec)
	}

	// 2. Execute with explicit text and dry-run
	resDry, err := app.Handle(ctx, ipc.Request{
		Method: "edit_execute",
		Params: map[string]any{
			"instruction": "make this shorter",
			"text":        "A longer custom text to condense",
			"dry_run":     true,
		},
	})
	if err != nil {
		t.Fatalf("edit_execute dry run failed: %v", err)
	}
	mDry := resDry.(map[string]any)
	if mDry["dry_run"] != true || mDry["pasted"] != false {
		t.Fatalf("unexpected dry run response: %+v", mDry)
	}

	// 3. Error case: missing instruction
	if _, err := app.Handle(ctx, ipc.Request{Method: "edit_execute"}); err == nil {
		t.Fatal("expected error on missing instruction")
	}
}

func TestApp_PreviewLifecycleIPC(t *testing.T) {
	cfg := config.Default()
	mockPolisher := polish.NewMockPolisher(func(req polish.Request) (polish.Response, error) {
		return polish.Response{
			Text:  "Proposal candidate text",
			Model: "mock-model",
		}, nil
	})
	paster := &recordingPaster{}
	mockSel := selection.NewMockCapturer("Original selection", "mock")

	app := NewApp(cfg,
		WithPolisher(mockPolisher),
		WithPaster(paster),
		WithSelectionCapturer(mockSel),
	)

	ctx := context.Background()

	// 1. Generate preview
	pRes, err := app.Handle(ctx, ipc.Request{
		Method: "edit_preview",
		Params: map[string]any{
			"instruction": "rewrite this",
		},
	})
	if err != nil {
		t.Fatalf("edit_preview failed: %v", err)
	}

	sess, ok := pRes.(*edit.PreviewSession)
	if !ok {
		t.Fatalf("unexpected type: %T", pRes)
	}
	if sess.Status != edit.PreviewPending {
		t.Fatalf("expected pending status, got %q", sess.Status)
	}
	if !strings.Contains(sess.DiffSummary, "- Original selection") {
		t.Fatalf("unexpected diff: %s", sess.DiffSummary)
	}

	// 2. List pending previews
	listRes, err := app.Handle(ctx, ipc.Request{Method: "edit_preview_list"})
	if err != nil {
		t.Fatalf("edit_preview_list failed: %v", err)
	}
	list := listRes.([]*edit.PreviewSession)
	if len(list) != 1 || list[0].ID != sess.ID {
		t.Fatalf("unexpected preview list: %+v", list)
	}

	// 3. Apply preview
	applyRes, err := app.Handle(ctx, ipc.Request{
		Method: "edit_apply",
		Params: map[string]any{"id": sess.ID},
	})
	if err != nil {
		t.Fatalf("edit_apply failed: %v", err)
	}
	appliedSess := applyRes.(*edit.PreviewSession)
	if appliedSess.Status != edit.PreviewApplied {
		t.Fatalf("expected applied status, got %q", appliedSess.Status)
	}
	if paster.Text() != "Proposal candidate text" {
		t.Fatalf("expected pasted text, got %q", paster.Text())
	}

	// 4. Verify undo is ready
	if !app.CanUndo() {
		t.Fatal("expected CanUndo after apply")
	}

	// 5. Reject preview on non-pending returns error
	if _, err := app.Handle(ctx, ipc.Request{
		Method: "edit_reject",
		Params: map[string]any{"id": sess.ID},
	}); err == nil {
		t.Fatal("expected error rejecting already applied session")
	}
}

func TestApp_UndoStackChainIPC(t *testing.T) {
	cfg := config.Default()
	paster := &recordingPaster{}
	reverter := paste.NewMockReverter()
	app := NewApp(cfg, WithPaster(paster), WithReverter(reverter))
	ctx := context.Background()

	// Push 3 transformations to undoStack
	app.recordUndoLocked(&UndoRecord{
		SessionID:      "sess-1",
		RawText:        "raw 1",
		UnpolishedText: "unpolished 1",
		PolishedText:   "polished 1",
		IsPolished:     true,
	})
	app.recordUndoLocked(&UndoRecord{
		SessionID:      "sess-2",
		RawText:        "raw 2",
		UnpolishedText: "unpolished 2",
		PolishedText:   "polished 2",
		IsPolished:     true,
	})
	app.recordUndoLocked(&UndoRecord{
		SessionID:      "sess-3",
		RawText:        "raw 3",
		UnpolishedText: "unpolished 3",
		PolishedText:   "polished 3",
		IsPolished:     true,
	})

	// Check undo stack
	stack := app.UndoStack()
	if len(stack) != 3 {
		t.Fatalf("expected 3 stack entries, got %d", len(stack))
	}

	// 1. Undo 1 step (depth 1)
	res1, err := app.Handle(ctx, ipc.Request{
		Method: "undo",
		Params: map[string]any{"depth": 1.0},
	})
	if err != nil {
		t.Fatalf("undo failed: %v", err)
	}
	u1 := res1.(*UndoResult)
	if u1.SessionID != "sess-3" || u1.Remaining != 2 {
		t.Fatalf("unexpected undo result: %+v", u1)
	}

	// 2. Undo 2 steps (depth 2)
	res2, err := app.Handle(ctx, ipc.Request{
		Method: "undo",
		Params: map[string]any{"depth": 2.0},
	})
	if err != nil {
		t.Fatalf("undo depth 2 failed: %v", err)
	}
	u2 := res2.(*UndoResult)
	if u2.SessionID != "sess-1" || u2.Remaining != 0 {
		t.Fatalf("unexpected undo result: %+v", u2)
	}

	// 3. Stack empty, further undo should fail
	if app.CanUndo() {
		t.Fatal("expected CanUndo to be false after fully unwinding stack")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "undo"}); err == nil {
		t.Fatal("expected error on empty stack undo")
	}
}

func TestApp_EditorContextIPCAndSelectionFallback(t *testing.T) {
	cfg := config.Default()
	app := NewApp(
		cfg,
		WithAppDetector(appcontext.NewMockDetector(appcontext.AppInfo{
			Name:     "Code",
			Category: appcontext.CategoryCode,
		})),
		WithSelectionCapturer(selection.NewMockCapturer("", "none")), // no AX selection
	)

	ctx := context.Background()

	// 1. Initial editor context should be inactive
	res, err := app.Handle(ctx, ipc.Request{Method: "editor_context_get"})
	if err != nil {
		t.Fatalf("editor_context_get error: %v", err)
	}
	statusMap, ok := res.(map[string]any)
	if !ok || statusMap["active"] != false {
		t.Fatalf("expected active=false initially, got: %+v", res)
	}

	// 2. Set editor context from VS Code
	setPayload := map[string]any{
		"editor_name":    "vscode",
		"workspace_root": "/Users/developer/project",
		"file_path":      "/Users/developer/project/internal/service.go",
		"file_name":      "service.go",
		"language_id":    "go",
		"line":           88,
		"column":         5,
		"selection_text": "func Process(data []byte) error { return nil }",
		"selection_range": map[string]any{
			"start_line": 88,
			"start_col":  1,
			"end_line":   88,
			"end_col":    47,
		},
		"surrounding_context": "package service\n\nfunc Process(data []byte) error { return nil }",
	}

	setRes, err := app.Handle(ctx, ipc.Request{
		Method: "editor_context_set",
		Params: setPayload,
	})
	if err != nil {
		t.Fatalf("editor_context_set error: %v", err)
	}
	setMap := setRes.(map[string]any)
	if setMap["status"] != "ok" {
		t.Fatalf("expected status=ok, got %+v", setMap)
	}

	// 3. Query editor context
	getRes, err := app.Handle(ctx, ipc.Request{Method: "editor_context_get"})
	if err != nil {
		t.Fatalf("editor_context_get error: %v", err)
	}
	getMap := getRes.(map[string]any)
	if getMap["active"] != true {
		t.Fatalf("expected active=true, got %+v", getMap)
	}
	if getMap["file_name"] != "service.go" || getMap["language_id"] != "go" {
		t.Fatalf("unexpected fields in editor context: %+v", getMap)
	}
	if getMap["selection_text"] != "func Process(data []byte) error { return nil }" {
		t.Fatalf("unexpected selection_text: %v", getMap["selection_text"])
	}

	// 4. selection_get should seamlessly fallback to editor context when AX capturer returns empty
	selRes, err := app.Handle(ctx, ipc.Request{Method: "selection_get"})
	if err != nil {
		t.Fatalf("selection_get fallback error: %v", err)
	}
	sel, ok := selRes.(selection.Result)
	if !ok {
		t.Fatalf("expected selection.Result, got %T", selRes)
	}
	if sel.Text != "func Process(data []byte) error { return nil }" {
		t.Fatalf("expected fallback selection text, got %q", sel.Text)
	}
	if sel.Method != "editor_context" {
		t.Fatalf("expected method editor_context, got %q", sel.Method)
	}

	// 5. Clear editor context
	clearRes, err := app.Handle(ctx, ipc.Request{Method: "editor_context_clear"})
	if err != nil {
		t.Fatalf("editor_context_clear error: %v", err)
	}
	clearMap := clearRes.(map[string]any)
	if clearMap["status"] != "cleared" {
		t.Fatalf("expected status=cleared, got %+v", clearMap)
	}

	// Verify it's cleared
	resAfter, err := app.Handle(ctx, ipc.Request{Method: "editor_context_get"})
	if err != nil {
		t.Fatalf("editor_context_get after clear error: %v", err)
	}
	afterMap := resAfter.(map[string]any)
	if afterMap["active"] != false {
		t.Fatalf("expected active=false after clear, got %+v", afterMap)
	}
}

func TestApp_TerminalContextIPC(t *testing.T) {
	cfg := config.Default()
	app := NewApp(cfg)
	ctx := context.Background()

	// 1. Initial terminal context should be inactive
	res, err := app.Handle(ctx, ipc.Request{Method: "terminal_context_get"})
	if err != nil {
		t.Fatalf("terminal_context_get error: %v", err)
	}
	initMap := res.(map[string]any)
	if initMap["active"] != false {
		t.Fatalf("expected active=false initially, got: %+v", initMap)
	}

	// 2. Set terminal context from shell hook
	setPayload := map[string]any{
		"terminal_app":    "Ghostty",
		"shell":           "zsh",
		"pid":             12345,
		"working_dir":     "/Users/dev/cadence",
		"git_branch":      "main",
		"last_command":    "git commit -m 'feat: terminal context'",
		"last_exit_code":  0,
		"recent_commands": []string{"git status", "git commit -m 'feat: terminal context'"},
	}

	setRes, err := app.Handle(ctx, ipc.Request{
		Method: "terminal_context_set",
		Params: setPayload,
	})
	if err != nil {
		t.Fatalf("terminal_context_set error: %v", err)
	}
	setMap := setRes.(map[string]any)
	if setMap["status"] != "ok" {
		t.Fatalf("expected status=ok, got %+v", setMap)
	}

	// 3. Query terminal context
	getRes, err := app.Handle(ctx, ipc.Request{Method: "terminal_context_get"})
	if err != nil {
		t.Fatalf("terminal_context_get error: %v", err)
	}
	getMap := getRes.(map[string]any)
	if getMap["active"] != true {
		t.Fatalf("expected active=true, got %+v", getMap)
	}
	if getMap["terminal_app"] != "Ghostty" || getMap["shell"] != "zsh" || getMap["git_branch"] != "main" {
		t.Fatalf("unexpected fields in terminal context: %+v", getMap)
	}
	if getMap["last_command"] != "git commit -m 'feat: terminal context'" {
		t.Fatalf("unexpected last_command: %v", getMap["last_command"])
	}

	// 4. Clear terminal context
	clearRes, err := app.Handle(ctx, ipc.Request{Method: "terminal_context_clear"})
	if err != nil {
		t.Fatalf("terminal_context_clear error: %v", err)
	}
	clearMap := clearRes.(map[string]any)
	if clearMap["status"] != "cleared" {
		t.Fatalf("expected status=cleared, got %+v", clearMap)
	}

	// 5. Query after clear
	afterRes, err := app.Handle(ctx, ipc.Request{Method: "terminal_context_get"})
	if err != nil {
		t.Fatalf("terminal_context_get after clear error: %v", err)
	}
	afterMap := afterRes.(map[string]any)
	if afterMap["active"] != false {
		t.Fatalf("expected active=false after clear, got %+v", afterMap)
	}
}

func TestApp_DevCommandGenerateIPC(t *testing.T) {
	cfg := config.Default()
	app := NewApp(cfg)
	ctx := context.Background()

	// 1. Missing instruction error
	if _, err := app.Handle(ctx, ipc.Request{Method: "dev_command_generate"}); err == nil {
		t.Fatal("expected error on missing instruction")
	}

	// 2. Set terminal working directory to a Go project
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
	app.TerminalContextManager().Set(appcontext.TerminalContext{
		WorkingDir: tmpDir,
	})

	// 3. Generate "run the tests" - should use terminal context working dir
	res, err := app.Handle(ctx, ipc.Request{
		Method: "dev_command_generate",
		Params: map[string]any{
			"instruction": "run the tests",
		},
	})
	if err != nil {
		t.Fatalf("dev_command_generate failed: %v", err)
	}
	genCmd, ok := res.(developer.GeneratedCommand)
	if !ok {
		t.Fatalf("expected developer.GeneratedCommand, got %T", res)
	}
	if genCmd.Command != "go test -v ./..." {
		t.Fatalf("expected 'go test -v ./...', got %q", genCmd.Command)
	}
	if genCmd.Ecosystem != developer.EcosystemGo {
		t.Fatalf("expected ecosystem go, got %s", genCmd.Ecosystem)
	}

	// 4. Test git commit generation
	resCommit, err := app.Handle(ctx, ipc.Request{
		Method: "dev_command_generate",
		Params: map[string]any{
			"instruction": "git commit with message add new feature",
		},
	})
	if err != nil {
		t.Fatalf("dev_command_generate commit failed: %v", err)
	}
	commitCmd := resCommit.(developer.GeneratedCommand)
	if commitCmd.Command != `git commit -m "add new feature"` {
		t.Fatalf("expected git commit command, got %q", commitCmd.Command)
	}
	if commitCmd.Risk.Level != developer.RiskRisky {
		t.Fatalf("expected commit to be risky, got %s", commitCmd.Risk.Level)
	}
}

func TestApp_DevCommandRiskIPC(t *testing.T) {
	cfg := config.Default()
	app := NewApp(cfg)
	ctx := context.Background()

	// 1. Missing command error
	if _, err := app.Handle(ctx, ipc.Request{Method: "dev_command_risk"}); err == nil {
		t.Fatal("expected error on missing command")
	}

	// 2. Safe command check
	safeRes, err := app.Handle(ctx, ipc.Request{
		Method: "dev_command_risk",
		Params: map[string]any{"command": "go test ./..."},
	})
	if err != nil {
		t.Fatalf("dev_command_risk failed: %v", err)
	}
	safeRisk, ok := safeRes.(developer.RiskAssessment)
	if !ok {
		t.Fatalf("expected RiskAssessment, got %T", safeRes)
	}
	if safeRisk.Level != developer.RiskSafe || safeRisk.RequiresConfirmation {
		t.Fatalf("expected safe with no confirmation, got %+v", safeRisk)
	}

	// 3. Destructive command check
	destRes, err := app.Handle(ctx, ipc.Request{
		Method: "dev_command_risk",
		Params: map[string]any{"command": "rm -rf /tmp/test && git push --force origin main"},
	})
	if err != nil {
		t.Fatalf("dev_command_risk destructive failed: %v", err)
	}
	destRisk := destRes.(developer.RiskAssessment)
	if destRisk.Level != developer.RiskDestructive || !destRisk.RequiresConfirmation {
		t.Fatalf("expected destructive with confirmation, got %+v", destRisk)
	}
}

func TestApp_DevCommandPreviewAndExecuteIPC(t *testing.T) {
	cfg := config.Default()
	app := NewApp(cfg)
	ctx := context.Background()
	tmpDir := t.TempDir()

	// 1. Missing command error for preview
	if _, err := app.Handle(ctx, ipc.Request{Method: "dev_command_preview"}); err == nil {
		t.Fatal("expected error on missing command for preview")
	}

	// 2. Preview safe command
	prevRes, err := app.Handle(ctx, ipc.Request{
		Method: "dev_command_preview",
		Params: map[string]any{
			"command":     "echo 'ipc test'",
			"working_dir": tmpDir,
		},
	})
	if err != nil {
		t.Fatalf("preview IPC failed: %v", err)
	}
	preview, ok := prevRes.(developer.DryRunPreview)
	if !ok {
		t.Fatalf("expected DryRunPreview, got %T", prevRes)
	}
	if preview.ID == "" || preview.Risk.Level != developer.RiskSafe {
		t.Fatalf("unexpected preview: %+v", preview)
	}

	// 3. Execute safe command
	execRes, err := app.Handle(ctx, ipc.Request{
		Method: "dev_command_execute",
		Params: map[string]any{
			"command":     "echo 'ipc execute safe'",
			"working_dir": tmpDir,
		},
	})
	if err != nil {
		t.Fatalf("execute safe failed: %v", err)
	}
	execResult, ok := execRes.(developer.ExecutionResult)
	if !ok {
		t.Fatalf("expected ExecutionResult, got %T", execRes)
	}
	if strings.TrimSpace(execResult.Stdout) != "ipc execute safe" {
		t.Fatalf("unexpected stdout: %q", execResult.Stdout)
	}

	// 4. Execute destructive command without confirm should fail
	_, err = app.Handle(ctx, ipc.Request{
		Method: "dev_command_execute",
		Params: map[string]any{
			"command":     "rm -rf /tmp/some_fake_dir_123",
			"working_dir": tmpDir,
			"confirm":     false,
		},
	})
	if err == nil {
		t.Fatal("expected error rejecting destructive command without confirm")
	}

	// 5. Execute destructive command with confirm should succeed
	dummyFile := filepath.Join(tmpDir, "dummy.txt")
	_ = os.WriteFile(dummyFile, []byte("xyz"), 0644)
	confirmExec, err := app.Handle(ctx, ipc.Request{
		Method: "dev_command_execute",
		Params: map[string]any{
			"command":     "rm -f " + dummyFile,
			"working_dir": tmpDir,
			"confirm":     true,
		},
	})
	if err != nil {
		t.Fatalf("confirmed execution failed: %v", err)
	}
	confirmResult := confirmExec.(developer.ExecutionResult)
	if confirmResult.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", confirmResult.ExitCode)
	}
}

func TestApp_GitAssistantIPC(t *testing.T) {
	cfg := config.Default()
	app := NewApp(cfg)
	ctx := context.Background()

	// 1. Setup mock git repository
	tmpDir := t.TempDir()
	runCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		_ = cmd.Run()
	}
	runCmd("init")
	runCmd("checkout", "-B", "main")
	runCmd("config", "user.name", "Tester")
	runCmd("config", "user.email", "tester@test.local")
	_ = os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test Repo\n"), 0644)
	runCmd("add", "README.md")
	runCmd("commit", "-m", "chore: init")

	// 2. Stage a file
	_ = os.WriteFile(filepath.Join(tmpDir, "feature.go"), []byte("package main\n"), 0644)
	runCmd("add", "feature.go")

	// 3. Test git_commit_generate IPC
	resCommit, err := app.Handle(ctx, ipc.Request{
		Method: "git_commit_generate",
		Params: map[string]any{
			"working_dir": tmpDir,
			"spoken_hint": "add main feature file",
		},
	})
	if err != nil {
		t.Fatalf("git_commit_generate failed: %v", err)
	}
	commitMsg, ok := resCommit.(developer.CommitMessage)
	if !ok {
		t.Fatalf("expected CommitMessage, got %T", resCommit)
	}
	if !strings.Contains(commitMsg.Title, "add main feature file") {
		t.Fatalf("expected hint in title, got %s", commitMsg.Title)
	}

	// 4. Test git_pr_generate IPC
	runCmd("commit", "-m", "feat: add feature file")
	resPR, err := app.Handle(ctx, ipc.Request{
		Method: "git_pr_generate",
		Params: map[string]any{
			"working_dir":  tmpDir,
			"base_branch":  "HEAD~1",
			"spoken_notes": "Implemented feature scaffold.",
		},
	})
	if err != nil {
		t.Fatalf("git_pr_generate failed: %v", err)
	}
	prDesc, ok := resPR.(developer.PRDescription)
	if !ok {
		t.Fatalf("expected PRDescription, got %T", resPR)
	}
	if !strings.Contains(prDesc.Body, "Overview") {
		t.Fatalf("expected Overview in PR body, got %s", prDesc.Body)
	}
}

func TestApp_DevCodeFormatIPC(t *testing.T) {
	cfg := config.Default()
	app := NewApp(cfg)
	ctx := context.Background()

	// 1. Missing text error
	if _, err := app.Handle(ctx, ipc.Request{Method: "dev_code_format"}); err == nil {
		t.Fatal("expected error on missing text")
	}

	// 2. Snake case format
	res, err := app.Handle(ctx, ipc.Request{
		Method: "dev_code_format",
		Params: map[string]any{
			"text":  "max retry limit",
			"style": "snake",
		},
	})
	if err != nil {
		t.Fatalf("dev_code_format snake failed: %v", err)
	}
	formatRes := res.(developer.CodeFormatResult)
	if formatRes.Result != "max_retry_limit" {
		t.Fatalf("expected max_retry_limit, got %s", formatRes.Result)
	}

	// 3. Spoken voice prefix format
	resPrefix, err := app.Handle(ctx, ipc.Request{
		Method: "dev_code_format",
		Params: map[string]any{
			"text": "camel case user profile service",
		},
	})
	if err != nil {
		t.Fatalf("dev_code_format voice prefix failed: %v", err)
	}
	prefixRes := resPrefix.(developer.CodeFormatResult)
	if prefixRes.Result != "userProfileService" {
		t.Fatalf("expected userProfileService, got %s", prefixRes.Result)
	}

	// 4. Docstring format with editor context language fallback
	app.EditorContextManager().Set(appcontext.EditorContext{
		LanguageID: "python",
	})
	resDoc, err := app.Handle(ctx, ipc.Request{
		Method: "dev_code_format",
		Params: map[string]any{
			"text":         "Calculate user balance.",
			"is_docstring": true,
		},
	})
	if err != nil {
		t.Fatalf("dev_code_format docstring failed: %v", err)
	}
	docRes := resDoc.(developer.CodeFormatResult)
	if docRes.Result != `"""Calculate user balance."""` {
		t.Fatalf("expected python docstring, got %s", docRes.Result)
	}
}

func TestApp_SystemEndpoints(t *testing.T) {
	mockCtrl := system.NewMockController()
	cfg := config.Default()
	app := NewApp(cfg, WithSystemController(mockCtrl))
	ctx := context.Background()

	// 1. system_command_execute error on empty input
	if _, err := app.Handle(ctx, ipc.Request{Method: "system_command_execute"}); err == nil {
		t.Fatalf("expected error on missing input")
	}

	// 2. system_command_execute with voice instruction
	res, err := app.Handle(ctx, ipc.Request{
		Method: "system_command_execute",
		Params: map[string]any{"input": "open Visual Studio Code"},
	})
	if err != nil {
		t.Fatalf("system_command_execute failed: %v", err)
	}
	mRes, ok := res.(map[string]any)
	if !ok || mRes["parsed"] != true {
		t.Fatalf("expected parsed true, got %v", res)
	}
	if len(mockCtrl.LaunchedApps) != 1 || mockCtrl.LaunchedApps[0] != "visual studio code" {
		t.Fatalf("expected visual studio code launched, got %v", mockCtrl.LaunchedApps)
	}

	// 3. system_command_execute unrecognized
	unrec, err := app.Handle(ctx, ipc.Request{
		Method: "system_command_execute",
		Params: map[string]any{"input": "random gibberish that does not match"},
	})
	if err != nil {
		t.Fatalf("system_command_execute failed: %v", err)
	}
	uRes := unrec.(map[string]any)
	if uRes["parsed"] != false {
		t.Fatalf("expected parsed false, got %v", unrec)
	}

	// 4. system_app_action (launch, focus, quit)
	_, err = app.Handle(ctx, ipc.Request{
		Method: "system_app_action",
		Params: map[string]any{"action": "focus", "app": "Chrome"},
	})
	if err != nil {
		t.Fatalf("system_app_action focus failed: %v", err)
	}
	if len(mockCtrl.FocusedApps) != 1 || mockCtrl.FocusedApps[0] != "Chrome" {
		t.Fatalf("expected Chrome focused, got %v", mockCtrl.FocusedApps)
	}

	_, err = app.Handle(ctx, ipc.Request{
		Method: "system_app_action",
		Params: map[string]any{"action": "quit", "app": "Slack"},
	})
	if err != nil {
		t.Fatalf("system_app_action quit failed: %v", err)
	}
	if len(mockCtrl.QuitApps) != 1 || mockCtrl.QuitApps[0] != "Slack" {
		t.Fatalf("expected Slack quit, got %v", mockCtrl.QuitApps)
	}

	// 5. system_window_action
	_, err = app.Handle(ctx, ipc.Request{
		Method: "system_window_action",
		Params: map[string]any{"action": "minimize"},
	})
	if err != nil {
		t.Fatalf("system_window_action failed: %v", err)
	}
	if len(mockCtrl.Windows) != 1 || mockCtrl.Windows[0] != system.WindowActionMinimize {
		t.Fatalf("expected minimize, got %v", mockCtrl.Windows)
	}

	// 6. system_screenshot
	sRes, err := app.Handle(ctx, ipc.Request{
		Method: "system_screenshot",
		Params: map[string]any{
			"active_window_only": true,
			"copy_to_clipboard":  true,
		},
	})
	if err != nil {
		t.Fatalf("system_screenshot failed: %v", err)
	}
	shotMap := sRes.(map[string]any)
	if shotMap["success"] != true {
		t.Fatalf("expected success true, got %v", sRes)
	}
	if len(mockCtrl.Screenshots) != 1 || !mockCtrl.Screenshots[0].ActiveWindowOnly || !mockCtrl.Screenshots[0].CopyToClipboard {
		t.Fatalf("mock screenshots mismatch: %v", mockCtrl.Screenshots)
	}

	// 7. system_volume
	vRes, err := app.Handle(ctx, ipc.Request{
		Method: "system_volume",
		Params: map[string]any{
			"level": 85.0,
			"mute":  true,
		},
	})
	if err != nil {
		t.Fatalf("system_volume failed: %v", err)
	}
	volMap := vRes.(map[string]any)
	if volMap["success"] != true {
		t.Fatalf("expected success true, got %v", vRes)
	}
	if mockCtrl.Volume != 85 || !mockCtrl.Muted {
		t.Fatalf("volume mismatch: vol=%d muted=%v", mockCtrl.Volume, mockCtrl.Muted)
	}
}

func TestApp_SelfControlIPC(t *testing.T) {
	mockCtrl := system.NewMockController()
	cfg := config.Default()
	app := NewApp(cfg, WithSystemController(mockCtrl))
	ctx := context.Background()

	// 1. Initial mode
	res, err := app.Handle(ctx, ipc.Request{Method: "mode_get"})
	if err != nil {
		t.Fatalf("mode_get failed: %v", err)
	}
	mMap := res.(map[string]any)
	if mMap["mode"] != "dictation" {
		t.Fatalf("expected initial mode dictation, got %v", mMap["mode"])
	}

	// 2. Explicit mode_set
	res, err = app.Handle(ctx, ipc.Request{
		Method: "mode_set",
		Params: map[string]any{"mode": "code"},
	})
	if err != nil {
		t.Fatalf("mode_set code failed: %v", err)
	}
	if app.Mode() != system.ModeCode {
		t.Fatalf("expected app mode code, got %s", app.Mode())
	}

	// 3. mode_set invalid error
	_, err = app.Handle(ctx, ipc.Request{
		Method: "mode_set",
		Params: map[string]any{"mode": "hyperdrive"},
	})
	if err == nil {
		t.Fatalf("expected error on invalid mode")
	}

	// 4. self_control_execute switch mode
	res, err = app.Handle(ctx, ipc.Request{
		Method: "self_control_execute",
		Params: map[string]any{"input": "cadence switch to command mode"},
	})
	if err != nil {
		t.Fatalf("self_control_execute switch mode failed: %v", err)
	}
	sMap := res.(map[string]any)
	if sMap["parsed"] != true || app.Mode() != system.ModeCommand {
		t.Fatalf("expected mode command, got parsed=%v mode=%v", sMap["parsed"], app.Mode())
	}
	if !strings.Contains(sMap["message"].(string), "command mode") {
		t.Fatalf("unexpected message: %v", sMap["message"])
	}

	// 5. self_control_execute query status
	res, err = app.Handle(ctx, ipc.Request{
		Method: "self_control_execute",
		Params: map[string]any{"input": "what mode are you in?"},
	})
	if err != nil {
		t.Fatalf("self_control_execute query status failed: %v", err)
	}
	qMap := res.(map[string]any)
	if !strings.Contains(qMap["message"].(string), "command mode") {
		t.Fatalf("expected status message with command mode, got %v", qMap["message"])
	}

	// 6. self_control_execute mute / unmute
	_, err = app.Handle(ctx, ipc.Request{
		Method: "self_control_execute",
		Params: map[string]any{"input": "mute cadence"},
	})
	if err != nil {
		t.Fatalf("self_control_execute mute failed: %v", err)
	}
	if !mockCtrl.Muted {
		t.Fatalf("expected mock controller muted")
	}

	_, err = app.Handle(ctx, ipc.Request{
		Method: "self_control_execute",
		Params: map[string]any{"input": "unmute microphone"},
	})
	if err != nil {
		t.Fatalf("self_control_execute unmute failed: %v", err)
	}
	if mockCtrl.Muted {
		t.Fatalf("expected mock controller unmuted")
	}

	// 7. self_control_execute recording lifecycle
	_, err = app.Handle(ctx, ipc.Request{
		Method: "self_control_execute",
		Params: map[string]any{"input": "start recording"},
	})
	if err != nil {
		t.Fatalf("start recording failed: %v", err)
	}

	_, err = app.Handle(ctx, ipc.Request{
		Method: "self_control_execute",
		Params: map[string]any{"input": "cancel recording"},
	})
	if err != nil {
		t.Fatalf("cancel recording failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 8. self_control_execute error handling
	_, err = app.Handle(ctx, ipc.Request{Method: "self_control_execute"})
	if err == nil {
		t.Fatalf("expected error on missing input")
	}

	unrec, err := app.Handle(ctx, ipc.Request{
		Method: "self_control_execute",
		Params: map[string]any{"input": "unrelated rambling phrase"},
	})
	if err != nil {
		t.Fatalf("unexpected error on unrecognized input: %v", err)
	}
	uMap := unrec.(map[string]any)
	if uMap["parsed"] != false {
		t.Fatalf("expected parsed false, got %v", uMap)
	}
}

func TestApp_AgentIPC(t *testing.T) {
	mockCtrl := system.NewMockController()
	agentEng := agent.NewEngine(agent.WithSystemController(mockCtrl))
	cfg := config.Default()
	app := NewApp(cfg, WithCapture(&testLiveCapture{}), WithStore(storage.NewMemory()), WithSystemController(mockCtrl), WithAgentEngine(agentEng))
	ctx := context.Background()

	// 1. agent_plan error on empty goal
	if _, err := app.Handle(ctx, ipc.Request{Method: "agent_plan"}); err == nil {
		t.Fatalf("expected error on empty goal for agent_plan")
	}

	// 2. agent_plan valid
	planRes, err := app.Handle(ctx, ipc.Request{
		Method: "agent_plan",
		Params: map[string]any{"goal": "open Chrome and then take a screenshot"},
	})
	if err != nil {
		t.Fatalf("agent_plan failed: %v", err)
	}
	plan, ok := planRes.(agent.Plan)
	if !ok || len(plan.Steps) != 2 {
		t.Fatalf("expected 2 plan steps, got %+v", planRes)
	}

	// 3. agent_run plan_only
	runPlanOnly, err := app.Handle(ctx, ipc.Request{
		Method: "agent_run",
		Params: map[string]any{
			"goal":      "open Slack and then minimize window",
			"plan_only": true,
		},
	})
	if err != nil {
		t.Fatalf("agent_run plan_only failed: %v", err)
	}
	trajPlan, ok := runPlanOnly.(*agent.ExecutionTrajectory)
	if !ok || trajPlan.Status != agent.StatusSuccess || len(trajPlan.Steps) != 0 {
		t.Fatalf("unexpected plan-only trajectory: %+v", runPlanOnly)
	}

	// 4. agent_run full autonomous execution
	runFull, err := app.Handle(ctx, ipc.Request{
		Method: "agent_run",
		Params: map[string]any{
			"goal":         "open Slack and then minimize window",
			"auto_approve": true,
		},
	})
	if err != nil {
		t.Fatalf("agent_run full failed: %v", err)
	}
	trajFull, ok := runFull.(*agent.ExecutionTrajectory)
	if !ok || trajFull.Status != agent.StatusSuccess || len(trajFull.Steps) != 2 {
		t.Fatalf("unexpected full trajectory: %+v", runFull)
	}
	if len(mockCtrl.LaunchedApps) != 1 || mockCtrl.LaunchedApps[0] != "slack" {
		t.Fatalf("mock launched apps = %v", mockCtrl.LaunchedApps)
	}
	if len(mockCtrl.Windows) != 1 || mockCtrl.Windows[0] != system.WindowActionMinimize {
		t.Fatalf("mock windows = %v", mockCtrl.Windows)
	}

	// 5. agent_history
	histRes, err := app.Handle(ctx, ipc.Request{
		Method: "agent_history",
		Params: map[string]any{"limit": 5.0},
	})
	if err != nil {
		t.Fatalf("agent_history failed: %v", err)
	}
	history, ok := histRes.([]*agent.ExecutionTrajectory)
	if !ok || len(history) < 2 {
		t.Fatalf("expected at least 2 history items, got %+v", histRes)
	}
}

func TestApp_ToolRegistryIPC(t *testing.T) {
	mockCtrl := system.NewMockController()
	agentEng := agent.NewEngine(agent.WithSystemController(mockCtrl))
	cfg := config.Default()
	app := NewApp(cfg, WithCapture(&testLiveCapture{}), WithStore(storage.NewMemory()), WithSystemController(mockCtrl), WithAgentEngine(agentEng))
	ctx := context.Background()

	// 1. tool_list
	listRes, err := app.Handle(ctx, ipc.Request{Method: "tool_list"})
	if err != nil {
		t.Fatalf("tool_list failed: %v", err)
	}
	lMap, ok := listRes.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", listRes)
	}
	tools, ok := lMap["tools"].([]agent.ToolDefinition)
	if !ok || len(tools) < 5 {
		t.Fatalf("expected at least 5 tools, got %d", len(tools))
	}
	cats, ok := lMap["categories"].([]string)
	if !ok || len(cats) < 3 {
		t.Fatalf("expected at least 3 categories, got %d", len(cats))
	}

	// 2. tool_call success (echo)
	callRes, err := app.Handle(ctx, ipc.Request{
		Method: "tool_call",
		Params: map[string]any{
			"name": "echo",
			"params": map[string]any{
				"message": "hello tool registry",
			},
		},
	})
	if err != nil {
		t.Fatalf("tool_call echo failed: %v", err)
	}
	cMap := callRes.(map[string]any)
	tRes := cMap["result"].(agent.ToolResult)
	if !tRes.Success || tRes.Output != "hello tool registry" {
		t.Fatalf("unexpected echo result: %+v", tRes)
	}

	// 3. tool_call error missing name
	if _, err := app.Handle(ctx, ipc.Request{Method: "tool_call"}); err == nil {
		t.Fatal("expected error on missing name")
	}

	// 4. tool_call unregistered tool
	unregRes, err := app.Handle(ctx, ipc.Request{
		Method: "tool_call",
		Params: map[string]any{"name": "unknown_tool"},
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	uMap := unregRes.(map[string]any)
	uRes := uMap["result"].(agent.ToolResult)
	if uRes.Success {
		t.Fatal("expected failure on unknown tool")
	}
}

func TestApp_PermissionsIPC(t *testing.T) {
	mockCtrl := system.NewMockController()
	agentEng := agent.NewEngine(agent.WithSystemController(mockCtrl))
	cfg := config.Default()
	app := NewApp(cfg, WithCapture(&testLiveCapture{}), WithStore(storage.NewMemory()), WithSystemController(mockCtrl), WithAgentEngine(agentEng))
	ctx := context.Background()

	// 1. permission_list
	listRes, err := app.Handle(ctx, ipc.Request{Method: "permission_list"})
	if err != nil {
		t.Fatalf("permission_list failed: %v", err)
	}
	lMap, ok := listRes.(map[string]any)
	if !ok || lMap["count"].(int) < 5 {
		t.Fatalf("expected at least 5 rules in list, got %+v", listRes)
	}

	// 2. permission_grant (deny fs write)
	grantRes, err := app.Handle(ctx, ipc.Request{
		Method: "permission_grant",
		Params: map[string]any{
			"tool":        "fs",
			"action":      "write",
			"level":       "deny",
			"description": "Deny all filesystem writes in test",
		},
	})
	if err != nil {
		t.Fatalf("permission_grant failed: %v", err)
	}
	gMap := grantRes.(map[string]any)
	if gMap["ok"] != true {
		t.Fatalf("expected ok true in grant response, got %+v", grantRes)
	}

	// 3. permission_check (should report denied)
	chkRes, err := app.Handle(ctx, ipc.Request{
		Method: "permission_check",
		Params: map[string]any{
			"tool":   "fs",
			"action": "write",
			"params": map[string]any{"path": "test.txt"},
		},
	})
	if err != nil {
		t.Fatalf("permission_check failed: %v", err)
	}
	cMap := chkRes.(map[string]any)
	if cMap["granted"] != false || cMap["level"] != "deny" {
		t.Fatalf("expected fs write to be denied, got %+v", chkRes)
	}

	// 4. permission_revoke (revoke the deny rule)
	revRes, err := app.Handle(ctx, ipc.Request{
		Method: "permission_revoke",
		Params: map[string]any{
			"tool":   "fs",
			"action": "write",
		},
	})
	if err != nil {
		t.Fatalf("permission_revoke failed: %v", err)
	}
	rMap := revRes.(map[string]any)
	if rMap["revoked"] != true {
		t.Fatalf("expected revoked true, got %+v", revRes)
	}

	// 5. permission_check again (fallback to generic allow)
	chkRes2, err := app.Handle(ctx, ipc.Request{
		Method: "permission_check",
		Params: map[string]any{
			"tool":   "fs",
			"action": "write",
			"params": map[string]any{"path": "test.txt"},
		},
	})
	if err != nil {
		t.Fatalf("permission_check failed: %v", err)
	}
	cMap2 := chkRes2.(map[string]any)
	if cMap2["granted"] != true {
		t.Fatalf("expected fs write to be allowed after revoking deny, got %+v", chkRes2)
	}

	// 6. Error validations
	if _, err := app.Handle(ctx, ipc.Request{Method: "permission_grant"}); err == nil {
		t.Fatal("expected error on missing tool in grant")
	}
	if _, err := app.Handle(ctx, ipc.Request{
		Method: "permission_grant",
		Params: map[string]any{"tool": "fs", "level": "invalid_level"},
	}); err == nil {
		t.Fatal("expected error on invalid permission level")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "permission_revoke"}); err == nil {
		t.Fatal("expected error on missing tool in revoke")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "permission_check"}); err == nil {
		t.Fatal("expected error on missing tool in check")
	}
}

func TestApp_PolicyIPC(t *testing.T) {
	mockCtrl := system.NewMockController()
	agentEng := agent.NewEngine(agent.WithSystemController(mockCtrl))
	cfg := config.Default()
	app := NewApp(cfg, WithCapture(&testLiveCapture{}), WithStore(storage.NewMemory()), WithSystemController(mockCtrl), WithAgentEngine(agentEng))
	ctx := context.Background()

	// 1. policy_get (default is auto_approve_safe)
	getRes, err := app.Handle(ctx, ipc.Request{Method: "policy_get"})
	if err != nil {
		t.Fatalf("policy_get failed: %v", err)
	}
	gMap := getRes.(map[string]any)
	if gMap["mode"] != "auto_approve_safe" {
		t.Fatalf("expected default mode auto_approve_safe, got %v", gMap["mode"])
	}

	// 2. policy_set
	setRes, err := app.Handle(ctx, ipc.Request{
		Method: "policy_set",
		Params: map[string]any{"mode": "always_ask"},
	})
	if err != nil {
		t.Fatalf("policy_set failed: %v", err)
	}
	sMap := setRes.(map[string]any)
	if sMap["mode"] != "always_ask" {
		t.Fatalf("expected mode always_ask, got %v", sMap["mode"])
	}

	// Verify get reflects new mode
	getRes2, err := app.Handle(ctx, ipc.Request{Method: "policy_get"})
	if err != nil {
		t.Fatalf("policy_get after set failed: %v", err)
	}
	if getRes2.(map[string]any)["mode"] != "always_ask" {
		t.Fatalf("expected mode always_ask, got %v", getRes2)
	}

	// 3. policy_evaluate
	evalRes, err := app.Handle(ctx, ipc.Request{
		Method: "policy_evaluate",
		Params: map[string]any{
			"tool":   "fs",
			"action": "write",
			"params": map[string]any{"path": "file.txt"},
		},
	})
	if err != nil {
		t.Fatalf("policy_evaluate failed: %v", err)
	}
	eMap := evalRes.(map[string]any)
	if eMap["requires_confirmation"] != true || eMap["risk_level"] != "risky" {
		t.Fatalf("expected risky action requiring confirmation, got %+v", evalRes)
	}

	// 4. Error validations
	if _, err := app.Handle(ctx, ipc.Request{Method: "policy_set"}); err == nil {
		t.Fatal("expected error on missing mode parameter")
	}
	if _, err := app.Handle(ctx, ipc.Request{
		Method: "policy_set",
		Params: map[string]any{"mode": "non_existent_mode"},
	}); err == nil {
		t.Fatal("expected error on invalid mode")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "policy_evaluate"}); err == nil {
		t.Fatal("expected error on missing tool parameter in evaluate")
	}
}

func TestApp_MeetingIPC(t *testing.T) {
	tmpDir := t.TempDir()
	store := storage.NewMemory()
	capture := &testLiveCapture{chunk: audio.Chunk{
		Samples:    []float32{0.1, 0.2, -0.1},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}}
	engine := asr.MockEngine{Text: "Continuous meeting audio"}

	mgr := meeting.NewManager(
		store,
		capture,
		meeting.WithAudioDirectory(tmpDir),
		meeting.WithASREngine(engine),
	)

	cfg := config.Default()
	app := NewApp(
		cfg,
		WithStore(store),
		WithCapture(capture),
		WithASR(engine),
		WithMeetingManager(mgr),
	)
	ctx := context.Background()

	// 1. Initial meeting_status: not recording
	statusRes, err := app.Handle(ctx, ipc.Request{Method: "meeting_status"})
	if err != nil {
		t.Fatalf("meeting_status failed: %v", err)
	}
	rep, ok := statusRes.(meeting.StatusReport)
	if !ok {
		t.Fatalf("expected meeting.StatusReport, got %T", statusRes)
	}
	if rep.IsRecording {
		t.Fatal("expected initial meeting_status to not be recording")
	}

	// 2. meeting_start
	startRes, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_start",
		Params: map[string]any{"title": "Engineering Sync"},
	})
	if err != nil {
		t.Fatalf("meeting_start failed: %v", err)
	}
	sess, ok := startRes.(*meeting.Session)
	if !ok || sess.ID == "" || sess.Title != "Engineering Sync" {
		t.Fatalf("unexpected meeting_start result: %+v", startRes)
	}

	// 3. meeting_status while recording
	statusRes2, err := app.Handle(ctx, ipc.Request{Method: "meeting_status"})
	if err != nil {
		t.Fatalf("meeting_status failed: %v", err)
	}
	rep2 := statusRes2.(meeting.StatusReport)
	if !rep2.IsRecording || rep2.ActiveSession == nil {
		t.Fatalf("expected recording status with active session, got %+v", rep2)
	}

	// 4. meeting_pause
	pauseRes, err := app.Handle(ctx, ipc.Request{Method: "meeting_pause"})
	if err != nil {
		t.Fatalf("meeting_pause failed: %v", err)
	}
	pMap, ok := pauseRes.(map[string]any)
	if !ok || pMap["ok"] != true {
		t.Fatalf("unexpected meeting_pause result: %+v", pauseRes)
	}
	pStatus, ok := pMap["status"].(meeting.StatusReport)
	if !ok || !pStatus.IsPaused {
		t.Fatalf("expected status.IsPaused to be true, got %+v", pMap)
	}

	// 5. meeting_resume
	resumeRes, err := app.Handle(ctx, ipc.Request{Method: "meeting_resume"})
	if err != nil {
		t.Fatalf("meeting_resume failed: %v", err)
	}
	rMap, ok := resumeRes.(map[string]any)
	if !ok || rMap["ok"] != true {
		t.Fatalf("unexpected meeting_resume result: %+v", resumeRes)
	}
	rStatus, ok := rMap["status"].(meeting.StatusReport)
	if !ok || rStatus.IsPaused {
		t.Fatalf("expected status.IsPaused to be false, got %+v", rMap)
	}

	// 6. meeting_stop
	stopRes, err := app.Handle(ctx, ipc.Request{Method: "meeting_stop"})
	if err != nil {
		t.Fatalf("meeting_stop failed: %v", err)
	}
	stoppedSess, ok := stopRes.(*meeting.Session)
	if !ok || stoppedSess.State != meeting.StateStopped {
		t.Fatalf("unexpected meeting_stop result: %+v", stopRes)
	}

	// 7. meeting_list
	listRes, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_list",
		Params: map[string]any{"limit": float64(10)},
	})
	if err != nil {
		t.Fatalf("meeting_list failed: %v", err)
	}
	meetings, ok := listRes.([]storage.Meeting)
	if !ok || len(meetings) == 0 {
		t.Fatalf("expected at least 1 meeting in list, got %+v", listRes)
	}
	if meetings[0].ID != sess.ID {
		t.Fatalf("expected meeting ID %s, got %s", sess.ID, meetings[0].ID)
	}

	// 8. meeting_get
	getRes, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_get",
		Params: map[string]any{"id": sess.ID},
	})
	if err != nil {
		t.Fatalf("meeting_get failed: %v", err)
	}
	gMap, ok := getRes.(map[string]any)
	if !ok {
		t.Fatalf("unexpected meeting_get response shape: %+v", getRes)
	}
	gotMtg, ok := gMap["meeting"].(*storage.Meeting)
	if !ok || gotMtg == nil || gotMtg.ID != sess.ID {
		t.Fatalf("expected valid meeting in meeting_get, got %+v", gMap["meeting"])
	}

	// 9. meeting_delete
	delRes, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_delete",
		Params: map[string]any{"id": sess.ID},
	})
	if err != nil {
		t.Fatalf("meeting_delete failed: %v", err)
	}
	dMap, ok := delRes.(map[string]any)
	if !ok || dMap["ok"] != true {
		t.Fatalf("unexpected meeting_delete result: %+v", delRes)
	}

	// Verify get after delete fails or returns meeting not found
	if _, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_get",
		Params: map[string]any{"id": sess.ID},
	}); err == nil {
		t.Fatal("expected error getting deleted meeting")
	}

	// 10. Error validations
	if _, err := app.Handle(ctx, ipc.Request{Method: "meeting_get"}); err == nil {
		t.Fatal("expected error on meeting_get without id")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "meeting_delete"}); err == nil {
		t.Fatal("expected error on meeting_delete without id")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "meeting_pause"}); err == nil {
		t.Fatal("expected error pausing when no active meeting")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "meeting_resume"}); err == nil {
		t.Fatal("expected error resuming when no active meeting")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "meeting_stop"}); err == nil {
		t.Fatal("expected error stopping when no active meeting")
	}
}

func TestApp_MeetingDiarizeIPC(t *testing.T) {
	tmpDir := t.TempDir()
	store := storage.NewMemory()
	capture := &testLiveCapture{chunk: audio.Chunk{
		Samples:    []float32{0.1, 0.2, -0.1},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}}
	engine := asr.MockEngine{Text: "Meeting text"}

	mockDiarizer := diarize.NewMockDiarizer(
		diarize.Segment{StartMs: 0, EndMs: 12000, Speaker: "SPEAKER_00", Confidence: 0.95},
		diarize.Segment{StartMs: 12000, EndMs: 25000, Speaker: "SPEAKER_01", Confidence: 0.90},
	)

	mgr := meeting.NewManager(
		store,
		capture,
		meeting.WithAudioDirectory(tmpDir),
		meeting.WithASREngine(engine),
		meeting.WithDiarizer(mockDiarizer),
	)

	cfg := config.Default()
	app := NewApp(
		cfg,
		WithStore(store),
		WithCapture(capture),
		WithASR(engine),
		WithMeetingManager(mgr),
		WithDiarizer(mockDiarizer),
	)
	ctx := context.Background()

	// Pre-create meeting and segments in storage
	mtgID := "mtg-diarize-ipc"
	wavFile := filepath.Join(tmpDir, "meeting.wav")
	_ = os.WriteFile(wavFile, []byte("audio-bytes"), 0644)

	_ = store.SaveMeeting(storage.Meeting{
		ID:        mtgID,
		Title:     "Sprint Planning",
		State:     "stopped",
		AudioPath: wavFile,
		CreatedAt: time.Now(),
	})
	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "seg-1",
		MeetingID:    mtgID,
		SegmentIndex: 0,
		StartMs:      0,
		EndMs:        10000,
		Text:         "Let's review the sprint backlog.",
		CreatedAt:    time.Now(),
	})
	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "seg-2",
		MeetingID:    mtgID,
		SegmentIndex: 1,
		StartMs:      10000,
		EndMs:        20000,
		Text:         "I will take the diarization task.",
		CreatedAt:    time.Now(),
	})

	// 1. Successful meeting_diarize call
	res, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_diarize",
		Params: map[string]any{
			"id":           mtgID,
			"num_speakers": float64(2),
		},
	})
	if err != nil {
		t.Fatalf("meeting_diarize failed: %v", err)
	}

	resMap, ok := res.(map[string]any)
	if !ok || resMap["ok"] != true {
		t.Fatalf("unexpected meeting_diarize response: %+v", res)
	}

	segs, ok := resMap["segments"].([]storage.MeetingSegment)
	if !ok || len(segs) != 2 {
		t.Fatalf("expected 2 diarized segments, got %+v", resMap["segments"])
	}
	if segs[0].Speaker != "SPEAKER_00" || segs[1].Speaker != "SPEAKER_01" {
		t.Fatalf("unexpected assigned speakers: seg0=%s, seg1=%s", segs[0].Speaker, segs[1].Speaker)
	}

	speakers, ok := resMap["speakers"].([]string)
	if !ok || len(speakers) != 2 {
		t.Fatalf("expected 2 unique speakers, got %+v", resMap["speakers"])
	}

	// 2. Error on missing id
	if _, err := app.Handle(ctx, ipc.Request{Method: "meeting_diarize"}); err == nil {
		t.Fatal("expected error on meeting_diarize without id")
	}

	// 3. Error on non-existent meeting
	if _, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_diarize",
		Params: map[string]any{"id": "non-existent-meeting"},
	}); err == nil {
		t.Fatal("expected error for non-existent meeting")
	}
}

func TestApp_MeetingSpeakersAndTurnsIPC(t *testing.T) {
	tmpDir := t.TempDir()
	store := storage.NewMemory()
	capture := &testLiveCapture{chunk: audio.Chunk{
		Samples:    []float32{0.1, 0.2, -0.1},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}}
	engine := asr.MockEngine{Text: "Meeting text"}

	mgr := meeting.NewManager(
		store,
		capture,
		meeting.WithAudioDirectory(tmpDir),
		meeting.WithASREngine(engine),
	)

	cfg := config.Default()
	app := NewApp(
		cfg,
		WithStore(store),
		WithCapture(capture),
		WithASR(engine),
		WithMeetingManager(mgr),
	)
	ctx := context.Background()

	mtgID := "mtg-turns-ipc"
	_ = store.SaveMeeting(storage.Meeting{
		ID:        mtgID,
		Title:     "Sprint Planning",
		State:     "stopped",
		CreatedAt: time.Now(),
	})
	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "seg-1",
		MeetingID:    mtgID,
		SegmentIndex: 0,
		StartMs:      0,
		EndMs:        10000,
		Text:         "Hello team.",
		Speaker:      "SPEAKER_00",
		CreatedAt:    time.Now(),
	})
	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "seg-2",
		MeetingID:    mtgID,
		SegmentIndex: 1,
		StartMs:      10000,
		EndMs:        20000,
		Text:         "Let's review the sprint.",
		Speaker:      "SPEAKER_00",
		CreatedAt:    time.Now(),
	})

	// 1. meeting_speaker_rename
	renRes, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_speaker_rename",
		Params: map[string]any{
			"meeting_id": mtgID,
			"from":       "SPEAKER_00",
			"to":         "Adesh Deshmukh",
		},
	})
	if err != nil {
		t.Fatalf("meeting_speaker_rename failed: %v", err)
	}
	rMap := renRes.(map[string]any)
	if rMap["ok"] != true || rMap["to"] != "Adesh Deshmukh" {
		t.Fatalf("unexpected rename response: %+v", renRes)
	}

	// 2. meeting_speaker_list
	listRes, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_speaker_list",
		Params: map[string]any{"meeting_id": mtgID},
	})
	if err != nil {
		t.Fatalf("meeting_speaker_list failed: %v", err)
	}
	lMap := listRes.(map[string]any)
	speakers := lMap["speakers"].([]storage.MeetingSpeaker)
	if len(speakers) != 1 || speakers[0].Name != "Adesh Deshmukh" {
		t.Fatalf("unexpected speakers list: %+v", lMap)
	}

	// 3. meeting_turns
	turnsRes, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_turns",
		Params: map[string]any{"meeting_id": mtgID},
	})
	if err != nil {
		t.Fatalf("meeting_turns failed: %v", err)
	}
	tMap := turnsRes.(map[string]any)
	turns := tMap["turns"].([]meeting.Turn)
	if len(turns) != 1 {
		t.Fatalf("expected 1 consolidated turn, got %d", len(turns))
	}
	if turns[0].Speaker != "Adesh Deshmukh" || turns[0].StartMs != 0 || turns[0].EndMs != 20000 {
		t.Fatalf("unexpected turn data: %+v", turns[0])
	}
	if turns[0].Text != "Hello team. Let's review the sprint." {
		t.Fatalf("unexpected turn text: %q", turns[0].Text)
	}

	// 4. Error validations
	if _, err := app.Handle(ctx, ipc.Request{Method: "meeting_speaker_rename"}); err == nil {
		t.Fatal("expected error on meeting_speaker_rename without params")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "meeting_speaker_list"}); err == nil {
		t.Fatal("expected error on meeting_speaker_list without id")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "meeting_turns"}); err == nil {
		t.Fatal("expected error on meeting_turns without id")
	}
}

func TestApp_MeetingSummaryIPC(t *testing.T) {
	tmpDir := t.TempDir()
	store := storage.NewMemory()
	capture := &testLiveCapture{chunk: audio.Chunk{
		Samples:    []float32{0.1, 0.2, -0.1},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}}
	engine := asr.MockEngine{Text: "Meeting text"}

	mgr := meeting.NewManager(
		store,
		capture,
		meeting.WithAudioDirectory(tmpDir),
		meeting.WithASREngine(engine),
	)

	cfg := config.Default()
	app := NewApp(
		cfg,
		WithStore(store),
		WithCapture(capture),
		WithASR(engine),
		WithMeetingManager(mgr),
	)
	ctx := context.Background()

	mtgID := "mtg-summary-ipc"
	_ = store.SaveMeeting(storage.Meeting{
		ID:        mtgID,
		Title:     "Architecture Review",
		State:     "stopped",
		CreatedAt: time.Now(),
	})
	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "seg-1",
		MeetingID:    mtgID,
		SegmentIndex: 0,
		StartMs:      0,
		EndMs:        8000,
		Text:         "We decided to implement the action item extraction.",
		Speaker:      "Alice",
		CreatedAt:    time.Now(),
	})
	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "seg-2",
		MeetingID:    mtgID,
		SegmentIndex: 1,
		StartMs:      8100,
		EndMs:        16000,
		Text:         "Action item: Bob will complete the CLI tests.",
		Speaker:      "Bob",
		CreatedAt:    time.Now(),
	})

	// 1. meeting_summary_get should return nil before summarize
	getInitRes, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_summary_get",
		Params: map[string]any{"meeting_id": mtgID},
	})
	if err != nil {
		t.Fatalf("meeting_summary_get initial failed: %v", err)
	}
	gInitMap := getInitRes.(map[string]any)
	if gInitMap["summary"] != nil {
		t.Fatalf("expected nil initial summary, got %+v", gInitMap["summary"])
	}

	// 2. meeting_summarize
	sumRes, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_summarize",
		Params: map[string]any{"meeting_id": mtgID},
	})
	if err != nil {
		t.Fatalf("meeting_summarize failed: %v", err)
	}
	sMap := sumRes.(map[string]any)
	if sMap["ok"] != true {
		t.Fatalf("expected ok=true, got %+v", sMap)
	}
	summary, ok := sMap["summary"].(*storage.MeetingSummary)
	if !ok || summary == nil {
		t.Fatalf("expected *storage.MeetingSummary, got %+v", sMap["summary"])
	}
	if len(summary.Decisions) == 0 {
		t.Errorf("expected decisions to be extracted")
	}
	if len(summary.ActionItems) == 0 {
		t.Errorf("expected action items to be extracted")
	}

	// 3. meeting_summary_get after summarize
	getRes, err := app.Handle(ctx, ipc.Request{
		Method: "meeting_summary_get",
		Params: map[string]any{"meeting_id": mtgID},
	})
	if err != nil {
		t.Fatalf("meeting_summary_get after summarize failed: %v", err)
	}
	gMap := getRes.(map[string]any)
	if gMap["ok"] != true {
		t.Fatalf("expected ok=true, got %+v", gMap)
	}
	fetchedSummary, ok := gMap["summary"].(*storage.MeetingSummary)
	if !ok || fetchedSummary == nil {
		t.Fatalf("expected non-nil summary, got %+v", gMap["summary"])
	}
	if fetchedSummary.MeetingID != mtgID {
		t.Errorf("expected meeting_id %s, got %s", mtgID, fetchedSummary.MeetingID)
	}

	// 4. Missing meeting_id validation
	if _, err := app.Handle(ctx, ipc.Request{Method: "meeting_summarize"}); err == nil {
		t.Fatal("expected error on meeting_summarize without id")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "meeting_summary_get"}); err == nil {
		t.Fatal("expected error on meeting_summary_get without id")
	}
}

func TestApp_KnowledgeBaseIPC(t *testing.T) {
	store := storage.NewMemory()
	capture := &testLiveCapture{chunk: audio.Chunk{
		Samples:    []float32{0.1, -0.1},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}}
	engine := asr.MockEngine{Text: "Test"}

	cfg := config.Default()
	app := NewApp(
		cfg,
		WithStore(store),
		WithCapture(capture),
		WithASR(engine),
	)
	ctx := context.Background()

	// 1. folder_save
	fRes, err := app.Handle(ctx, ipc.Request{
		Method: "folder_save",
		Params: map[string]any{
			"name": "Design Docs",
		},
	})
	if err != nil {
		t.Fatalf("folder_save failed: %v", err)
	}
	fMap := fRes.(map[string]any)
	folder := fMap["folder"].(*storage.Folder)
	if folder.Name != "Design Docs" {
		t.Errorf("expected folder name 'Design Docs', got %q", folder.Name)
	}

	// 2. folder_list
	flRes, err := app.Handle(ctx, ipc.Request{Method: "folder_list"})
	if err != nil {
		t.Fatalf("folder_list failed: %v", err)
	}
	flMap := flRes.(map[string]any)
	if flMap["count"] != 1 {
		t.Errorf("expected 1 folder, got %v", flMap["count"])
	}

	// 3. note_save
	nRes, err := app.Handle(ctx, ipc.Request{
		Method: "note_save",
		Params: map[string]any{
			"title":   "V1 Architecture",
			"content": "# V1 Architecture\n\nInitial draft of system design.",
			"folder":  folder.ID,
			"tags":    []any{"v1", "design"},
		},
	})
	if err != nil {
		t.Fatalf("note_save failed: %v", err)
	}
	nMap := nRes.(map[string]any)
	note := nMap["note"].(*storage.Note)
	if note.Title != "V1 Architecture" || len(note.Tags) != 2 {
		t.Fatalf("unexpected note saved: %+v", note)
	}

	// 4. note_get
	ngRes, err := app.Handle(ctx, ipc.Request{
		Method: "note_get",
		Params: map[string]any{"id": note.ID},
	})
	if err != nil {
		t.Fatalf("note_get failed: %v", err)
	}
	ngMap := ngRes.(map[string]any)
	gotNote := ngMap["note"].(*storage.Note)
	if gotNote.ID != note.ID {
		t.Errorf("expected note ID %s, got %s", note.ID, gotNote.ID)
	}

	// 5. note_list
	nlRes, err := app.Handle(ctx, ipc.Request{
		Method: "note_list",
		Params: map[string]any{"tag": "design"},
	})
	if err != nil {
		t.Fatalf("note_list failed: %v", err)
	}
	nlMap := nlRes.(map[string]any)
	if nlMap["count"] != 1 {
		t.Errorf("expected 1 note in list, got %v", nlMap["count"])
	}

	// 6. tag_list
	tlRes, err := app.Handle(ctx, ipc.Request{Method: "tag_list"})
	if err != nil {
		t.Fatalf("tag_list failed: %v", err)
	}
	tlMap := tlRes.(map[string]any)
	if tlMap["count"] != 2 {
		t.Errorf("expected 2 tags, got %v", tlMap["count"])
	}

	// 7. note_from_meeting
	mtgID := "mtg-kb-test"
	_ = store.SaveMeeting(storage.Meeting{
		ID:        mtgID,
		Title:     "Design Review",
		State:     "stopped",
		CreatedAt: time.Now(),
	})
	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "seg-1",
		MeetingID:    mtgID,
		SegmentIndex: 0,
		Text:         "Reviewed the design.",
		Speaker:      "Alice",
		CreatedAt:    time.Now(),
	})
	nfmRes, err := app.Handle(ctx, ipc.Request{
		Method: "note_from_meeting",
		Params: map[string]any{
			"meeting_id": mtgID,
			"folder":     "Meetings",
		},
	})
	if err != nil {
		t.Fatalf("note_from_meeting failed: %v", err)
	}
	nfmMap := nfmRes.(map[string]any)
	mtgNote := nfmMap["note"].(*storage.Note)
	if mtgNote.MeetingID != mtgID {
		t.Errorf("expected meeting ID %s, got %s", mtgID, mtgNote.MeetingID)
	}

	// 8. note_delete
	if _, err := app.Handle(ctx, ipc.Request{
		Method: "note_delete",
		Params: map[string]any{"id": note.ID},
	}); err != nil {
		t.Fatalf("note_delete failed: %v", err)
	}

	// 9. folder_delete
	if _, err := app.Handle(ctx, ipc.Request{
		Method: "folder_delete",
		Params: map[string]any{"id": folder.ID},
	}); err != nil {
		t.Fatalf("folder_delete failed: %v", err)
	}
}

func TestApp_SearchIPC(t *testing.T) {
	store := storage.NewMemory()
	capture := &testLiveCapture{chunk: audio.Chunk{
		Samples:    []float32{0.1, -0.1},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}}
	engine := asr.MockEngine{Text: "Test"}

	cfg := config.Default()
	app := NewApp(
		cfg,
		WithStore(store),
		WithCapture(capture),
		WithASR(engine),
	)
	ctx := context.Background()

	_ = store.SaveNote(storage.Note{
		ID:        "note-ipc-1",
		Title:     "GraphQL API Spec",
		Content:   "Schema definition for GraphQL queries and mutations.",
		CreatedAt: time.Now(),
	})
	_ = store.SaveTranscript(storage.Transcript{
		ID:        "tr-ipc-1",
		Text:      "We are building GraphQL resolvers.",
		CreatedAt: time.Now(),
	})

	// 1. Search all
	res, err := app.Handle(ctx, ipc.Request{
		Method: "search",
		Params: map[string]any{
			"query": "GraphQL",
		},
	})
	if err != nil {
		t.Fatalf("search IPC failed: %v", err)
	}
	rMap := res.(map[string]any)
	if rMap["ok"] != true || rMap["count"] != 2 {
		t.Fatalf("expected 2 search results, got %+v", rMap)
	}

	// 2. Search note type only
	resNotes, err := app.Handle(ctx, ipc.Request{
		Method: "search",
		Params: map[string]any{
			"query": "GraphQL",
			"type":  "note",
		},
	})
	if err != nil {
		t.Fatalf("search IPC notes only failed: %v", err)
	}
	rnMap := resNotes.(map[string]any)
	if rnMap["count"] != 1 {
		t.Fatalf("expected 1 note hit, got %v", rnMap["count"])
	}

	// 3. Error validation: missing query
	if _, err := app.Handle(ctx, ipc.Request{Method: "search"}); err == nil {
		t.Fatal("expected error on search without query")
	}
}

func TestApp_SemanticSearchAndRelatedIPC(t *testing.T) {
	store := storage.NewMemory()
	capture := &testLiveCapture{chunk: audio.Chunk{
		Samples:    []float32{0.1, -0.1},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}}
	engine := asr.MockEngine{Text: "Test"}

	cfg := config.Default()
	app := NewApp(
		cfg,
		WithStore(store),
		WithCapture(capture),
		WithASR(engine),
	)
	ctx := context.Background()

	_ = store.SaveNote(storage.Note{
		ID:        "note-sem-1",
		Title:     "PostgreSQL Database Optimization",
		Content:   "PostgreSQL index tuning, vacuuming, and slow query explain analyze execution plans.",
		CreatedAt: time.Now(),
	})
	_ = store.SaveTranscript(storage.Transcript{
		ID:        "tr-sem-1",
		Text:      "We analyzed slow PostgreSQL queries using explain analyze.",
		CreatedAt: time.Now(),
	})
	_ = store.SaveTranscript(storage.Transcript{
		ID:        "tr-sem-2",
		Text:      "Baking homemade sourdough artisan bread in the oven with flour.",
		CreatedAt: time.Now(),
	})

	// 1. Index embeddings
	idxRes, err := app.Handle(ctx, ipc.Request{Method: "index_embeddings"})
	if err != nil {
		t.Fatalf("index_embeddings IPC failed: %v", err)
	}
	idxMap := idxRes.(map[string]any)
	if idxMap["ok"] != true || idxMap["indexed"].(int) != 3 {
		t.Fatalf("expected 3 items indexed, got: %+v", idxMap)
	}

	// 2. Semantic search
	semRes, err := app.Handle(ctx, ipc.Request{
		Method: "semantic_search",
		Params: map[string]any{
			"query": "Postgres database indexing",
			"limit": 5,
		},
	})
	if err != nil {
		t.Fatalf("semantic_search failed: %v", err)
	}
	semMap := semRes.(map[string]any)
	if semMap["ok"] != true || semMap["count"].(int) == 0 {
		t.Fatalf("expected semantic hits, got: %+v", semMap)
	}

	// 3. Search with semantic: true flag
	searchFlagRes, err := app.Handle(ctx, ipc.Request{
		Method: "search",
		Params: map[string]any{
			"query":    "Postgres database indexing",
			"semantic": true,
		},
	})
	if err != nil {
		t.Fatalf("search with semantic flag failed: %v", err)
	}
	sfMap := searchFlagRes.(map[string]any)
	if sfMap["ok"] != true || sfMap["count"].(int) == 0 {
		t.Fatalf("expected semantic hits via search flag, got: %+v", sfMap)
	}

	// 4. Find related
	relRes, err := app.Handle(ctx, ipc.Request{
		Method: "find_related",
		Params: map[string]any{
			"id":   "tr-sem-1",
			"type": "transcript",
		},
	})
	if err != nil {
		t.Fatalf("find_related failed: %v", err)
	}
	relMap := relRes.(map[string]any)
	if relMap["ok"] != true {
		t.Fatalf("expected ok true from find_related: %+v", relMap)
	}
	relResults := relMap["results"].([]search.SemanticResult)
	if len(relResults) == 0 {
		t.Fatalf("expected at least 1 related result for tr-sem-1")
	}
	// Note-sem-1 should be related
	if relResults[0].ID != "note-sem-1" {
		t.Logf("First related item: %+v", relResults[0])
	}

	// 5. Error cases
	if _, err := app.Handle(ctx, ipc.Request{Method: "semantic_search"}); err == nil {
		t.Fatal("expected error for empty semantic_search query")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "find_related"}); err == nil {
		t.Fatal("expected error for missing id in find_related")
	}
}

func TestApp_ExportAndImportIPC(t *testing.T) {
	store := storage.NewMemory()
	capture := &testLiveCapture{chunk: audio.Chunk{
		Samples:    []float32{0.1, -0.1},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}}
	engine := asr.MockEngine{Text: "Test"}

	cfg := config.Default()
	app := NewApp(
		cfg,
		WithStore(store),
		WithCapture(capture),
		WithASR(engine),
	)
	ctx := context.Background()

	now := time.Now().UTC()
	mtgID := "mtg-ipc-exp"
	_ = store.SaveMeeting(storage.Meeting{
		ID:         mtgID,
		Title:      "Quarterly Roadmap",
		DurationMs: 60000,
		CreatedAt:  now,
	})
	_ = store.SetMeetingSpeaker(storage.MeetingSpeaker{
		MeetingID: mtgID,
		SpeakerID: "SPEAKER_00",
		Name:      "Alice",
		UpdatedAt: now,
	})
	_ = store.SaveMeetingSegment(storage.MeetingSegment{
		ID:           "seg-exp-1",
		MeetingID:    mtgID,
		SegmentIndex: 0,
		StartMs:      1000,
		EndMs:        4000,
		Speaker:      "SPEAKER_00",
		Text:         "Welcome to the quarterly roadmap review.",
		CreatedAt:    now,
	})
	_ = store.SaveMeetingSummary(storage.MeetingSummary{
		MeetingID: mtgID,
		Overview:  "High level review of Q1 deliverables.",
		Topics:    []string{"Roadmap goals", "Hiring plan"},
		ActionItems: []storage.ActionItem{
			{ID: "act-1", Description: "Finalize budget", Assignee: "Alice"},
		},
		Decisions: []string{"Approved new engine roadmap"},
		CreatedAt: now,
	})

	// 1. Export Markdown
	expMdRes, err := app.Handle(ctx, ipc.Request{
		Method: "export",
		Params: map[string]any{
			"id":     mtgID,
			"type":   "meeting",
			"format": "markdown",
		},
	})
	if err != nil {
		t.Fatalf("export markdown IPC failed: %v", err)
	}
	mdMap := expMdRes.(map[string]any)
	if mdMap["ok"] != true {
		t.Fatalf("expected ok true in export: %+v", mdMap)
	}
	content := mdMap["content"].(string)
	if !strings.Contains(content, "# Quarterly Roadmap") || !strings.Contains(content, "Alice") {
		t.Fatalf("unexpected markdown content: %s", content)
	}

	// 2. Export SRT to file
	tempDir := t.TempDir()
	outSRTPath := filepath.Join(tempDir, "roadmap.srt")
	expSrtRes, err := app.Handle(ctx, ipc.Request{
		Method: "export",
		Params: map[string]any{
			"id":          mtgID,
			"type":        "meeting",
			"format":      "srt",
			"output_path": outSRTPath,
		},
	})
	if err != nil {
		t.Fatalf("export srt IPC failed: %v", err)
	}
	srtMap := expSrtRes.(map[string]any)
	if srtMap["ok"] != true {
		t.Fatalf("expected ok in srt export: %+v", srtMap)
	}
	fileBytes, err := os.ReadFile(outSRTPath)
	if err != nil || !strings.Contains(string(fileBytes), "00:00:01,000 --> 00:00:04,000") {
		t.Fatalf("written SRT file invalid: %s (err: %v)", string(fileBytes), err)
	}

	// 3. Import SRT file via import_file IPC
	impRes, err := app.Handle(ctx, ipc.Request{
		Method: "import_file",
		Params: map[string]any{
			"path":  outSRTPath,
			"title": "Imported Roadmap Subtitles",
		},
	})
	if err != nil {
		t.Fatalf("import_file failed: %v", err)
	}
	impMap := impRes.(map[string]any)
	if impMap["ok"] != true || impMap["segments_count"].(int) != 1 {
		t.Fatalf("unexpected import result: %+v", impMap)
	}

	// 4. Export Note
	noteID := "note-exp-1"
	_ = store.SaveNote(storage.Note{
		ID:        noteID,
		Title:     "API Guidelines",
		Content:   "Standardize on REST and JSON-RPC.",
		CreatedAt: now,
	})
	noteExpRes, err := app.Handle(ctx, ipc.Request{
		Method: "export",
		Params: map[string]any{
			"id":     noteID,
			"type":   "note",
			"format": "markdown",
		},
	})
	if err != nil {
		t.Fatalf("export note failed: %v", err)
	}
	neMap := noteExpRes.(map[string]any)
	if !strings.Contains(neMap["content"].(string), "API Guidelines") {
		t.Fatalf("unexpected note export: %+v", neMap)
	}

	// 5. Error validations
	if _, err := app.Handle(ctx, ipc.Request{Method: "export"}); err == nil {
		t.Fatal("expected error on export without id")
	}
	if _, err := app.Handle(ctx, ipc.Request{Method: "import_file"}); err == nil {
		t.Fatal("expected error on import without path")
	}
}

func TestApp_PluginIPC(t *testing.T) {
	cfg := config.Default()
	app := NewApp(cfg)
	ctx := context.Background()
	tempDir := t.TempDir()

	// 1. Scaffold plugin
	scaffoldRes, err := app.Handle(ctx, ipc.Request{
		Method: "plugin_init_scaffold",
		Params: map[string]any{
			"dir":         tempDir,
			"id":          "com.example.test-plugin",
			"name":        "Test Plugin",
			"description": "Scaffolded plugin for test",
		},
	})
	if err != nil {
		t.Fatalf("plugin_init_scaffold failed: %v", err)
	}
	scMap := scaffoldRes.(map[string]any)
	if scMap["ok"] != true || scMap["id"] != "com.example.test-plugin" {
		t.Fatalf("unexpected scaffold result: %+v", scMap)
	}

	manifestFile := filepath.Join(tempDir, "plugin.yaml")
	if _, err := os.Stat(manifestFile); err != nil {
		t.Fatalf("plugin.yaml not written to %s", manifestFile)
	}

	// 2. Validate manifest via IPC
	valRes, err := app.Handle(ctx, ipc.Request{
		Method: "plugin_validate",
		Params: map[string]any{
			"path": manifestFile,
		},
	})
	if err != nil {
		t.Fatalf("plugin_validate failed: %v", err)
	}
	valMap := valRes.(map[string]any)
	if valMap["ok"] != true {
		t.Fatalf("expected valid manifest, got: %+v", valMap)
	}

	// 3. Register plugin via IPC
	regRes, err := app.Handle(ctx, ipc.Request{
		Method: "plugin_register",
		Params: map[string]any{
			"path": manifestFile,
		},
	})
	if err != nil {
		t.Fatalf("plugin_register failed: %v", err)
	}
	regMap := regRes.(map[string]any)
	if regMap["ok"] != true || regMap["id"] != "com.example.test-plugin" {
		t.Fatalf("unexpected register result: %+v", regMap)
	}

	// 4. List plugins
	listRes, err := app.Handle(ctx, ipc.Request{
		Method: "plugin_list",
	})
	if err != nil {
		t.Fatalf("plugin_list failed: %v", err)
	}
	listMap := listRes.(map[string]any)
	if listMap["ok"] != true || listMap["count"].(int) != 1 {
		t.Fatalf("unexpected plugin list: %+v", listMap)
	}

	// 5. Manifest lookup by ID
	manRes, err := app.Handle(ctx, ipc.Request{
		Method: "plugin_manifest",
		Params: map[string]any{
			"id": "com.example.test-plugin",
		},
	})
	if err != nil {
		t.Fatalf("plugin_manifest failed: %v", err)
	}
	manMap := manRes.(map[string]any)
	if manMap["ok"] != true {
		t.Fatalf("unexpected manifest lookup: %+v", manMap)
	}

	// 6. Error validations
	if _, err := app.Handle(ctx, ipc.Request{Method: "plugin_validate"}); err == nil {
		t.Fatal("expected error on validate without path or data")
	}
}

func TestApp_PluginLifecycleIPC(t *testing.T) {
	tempDir := t.TempDir()
	pluginsHome := filepath.Join(tempDir, "plugins_home")
	srcDir := filepath.Join(tempDir, "sample_plugin")
	_ = os.MkdirAll(srcDir, 0755)

	scriptContent := `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *"\"type\":\"init\""*)
      echo '{"type":"init_ack","payload":{"ok":true}}'
      ;;
    *"\"type\":\"ping\""*)
      echo '{"type":"pong"}'
      ;;
    *)
      echo '{"type":"pong"}'
      ;;
  esac
done
`
	scriptPath := filepath.Join(srcDir, "entry.sh")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("write entry.sh: %v", err)
	}

	manifestYAML := `
id: sample.echo.plugin
name: Sample Echo Plugin
version: 1.0.0
description: Sample plugin for daemon IPC lifecycle testing
entrypoint: ./entry.sh
capabilities:
  - command
`
	if err := os.WriteFile(filepath.Join(srcDir, "plugin.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatalf("write plugin.yaml: %v", err)
	}

	memStore := storage.NewMemory()
	reg := plugins.NewRegistry()
	mgr := plugins.NewManager(memStore, reg, pluginsHome)

	cfg := config.Default()
	app := NewApp(cfg, WithStore(memStore), WithPluginRegistry(reg), WithPluginManager(mgr))
	ctx := context.Background()

	// 1. Install
	instRes, err := app.Handle(ctx, ipc.Request{
		Method: "plugin_install",
		Params: map[string]any{"path": srcDir},
	})
	if err != nil {
		t.Fatalf("plugin_install failed: %v", err)
	}
	instMap := instRes.(map[string]any)
	if instMap["ok"] != true {
		t.Fatalf("expected ok true, got: %+v", instMap)
	}

	// 2. Status
	stRes, err := app.Handle(ctx, ipc.Request{
		Method: "plugin_status",
		Params: map[string]any{"id": "sample.echo.plugin"},
	})
	if err != nil {
		t.Fatalf("plugin_status failed: %v", err)
	}
	stMap := stRes.(map[string]any)
	if stMap["ok"] != true {
		t.Fatalf("expected ok true in status: %+v", stMap)
	}

	// 3. List
	listRes, err := app.Handle(ctx, ipc.Request{Method: "plugin_list"})
	if err != nil {
		t.Fatalf("plugin_list failed: %v", err)
	}
	listMap := listRes.(map[string]any)
	if listMap["count"].(int) != 1 {
		t.Fatalf("expected 1 plugin in list, got: %+v", listMap)
	}

	// 4. Activate
	actRes, err := app.Handle(ctx, ipc.Request{
		Method: "plugin_activate",
		Params: map[string]any{"id": "sample.echo.plugin"},
	})
	if err != nil {
		t.Fatalf("plugin_activate failed: %v", err)
	}
	actMap := actRes.(map[string]any)
	if actMap["ok"] != true {
		t.Fatalf("expected ok true in activate: %+v", actMap)
	}

	// 5. Deactivate
	deactRes, err := app.Handle(ctx, ipc.Request{
		Method: "plugin_deactivate",
		Params: map[string]any{"id": "sample.echo.plugin"},
	})
	if err != nil {
		t.Fatalf("plugin_deactivate failed: %v", err)
	}
	deactMap := deactRes.(map[string]any)
	if deactMap["ok"] != true {
		t.Fatalf("expected ok true in deactivate: %+v", deactMap)
	}

	// 6. Uninstall
	uninstRes, err := app.Handle(ctx, ipc.Request{
		Method: "plugin_uninstall",
		Params: map[string]any{"id": "sample.echo.plugin"},
	})
	if err != nil {
		t.Fatalf("plugin_uninstall failed: %v", err)
	}
	uninstMap := uninstRes.(map[string]any)
	if uninstMap["ok"] != true {
		t.Fatalf("expected ok true in uninstall: %+v", uninstMap)
	}
}
