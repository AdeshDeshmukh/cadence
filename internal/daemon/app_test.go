package daemon

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/config"
	"github.com/AdeshDeshmukh/cadence/internal/hotkey"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
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

