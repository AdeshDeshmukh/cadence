package daemon

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/config"
	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
	"github.com/AdeshDeshmukh/cadence/internal/edit"
	"github.com/AdeshDeshmukh/cadence/internal/hotkey"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/polish"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
	"github.com/AdeshDeshmukh/cadence/internal/selection"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
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
