package integration_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/config"
	"github.com/AdeshDeshmukh/cadence/internal/daemon"
	"github.com/AdeshDeshmukh/cadence/internal/hotkey"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

type mockPaster struct {
	pastedText string
}

func (m *mockPaster) Paste(text string) (paste.Result, error) {
	m.pastedText = text
	return paste.Result{Method: "mock", Success: true}, nil
}

type controllableCapture struct {
	mu     sync.Mutex
	stopCh chan struct{}
	chunk  audio.Chunk
}

func (c *controllableCapture) Devices() ([]audio.Device, error) { return nil, nil }

func (c *controllableCapture) Start() (<-chan audio.Chunk, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopCh = make(chan struct{})
	ch := make(chan audio.Chunk, 10)
	ch <- c.chunk
	go func(stopCh chan struct{}) {
		<-stopCh
		close(ch)
	}(c.stopCh)
	return ch, nil
}

func (c *controllableCapture) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopCh != nil {
		select {
		case <-c.stopCh:
		default:
			close(c.stopCh)
		}
	}
	return nil
}

func TestDaemonEndToEndDictation(t *testing.T) {
	cfg := config.Default()
	cfg.Hotkey.Mode = "push_to_talk"

	simHotkey := hotkey.NewSimulatedListener("Cmd+Shift+Space", hotkey.ModePushToTalk)
	paster := &mockPaster{}
	store := storage.NewMemory()

	chunk := audio.Chunk{
		Samples:    []float32{0.05, -0.05, 0.1},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}
	capture := &controllableCapture{chunk: chunk}
	engine := asr.MockEngine{Text: "um cadence speech to text exclamation mark"}

	pipe := postprocess.New(
		postprocess.NewFillerRemoval(),
		postprocess.NewPunctuation(),
	)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind IPC listener: %v", err)
	}
	defer listener.Close()

	app := daemon.NewApp(
		cfg,
		WithCapture(capture),
		WithASR(engine),
		WithPaster(paster),
		WithStore(store),
		WithPostprocess(pipe),
		WithHotkeyListener(simHotkey),
		WithIPCListener(listener),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("app Start failed: %v", err)
	}
	defer app.Stop()

	// 1. Verify IPC server status
	client := ipc.Client{
		Network: "tcp",
		Address: listener.Addr().String(),
	}
	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()

	statusResp, err := client.Call(callCtx, "status", nil)
	if err != nil {
		t.Fatalf("IPC status query failed: %v", err)
	}
	statusMap, ok := statusResp.(map[string]any)
	if !ok || statusMap["status"] != "running" {
		t.Fatalf("unexpected status response: %v", statusResp)
	}

	// 2. User presses global hotkey (Push-to-Talk KeyDown)
	simHotkey.Emit(hotkey.EventKeyDown)
	time.Sleep(50 * time.Millisecond)
	if !app.IsRecording() {
		t.Fatal("expected daemon to enter recording state on hotkey press")
	}

	// 3. User releases hotkey (Push-to-Talk KeyUp)
	simHotkey.Emit(hotkey.EventKeyUp)

	// Wait for pipeline completion (transcribe -> postprocess -> paste cascade -> store)
	time.Sleep(200 * time.Millisecond)

	if app.IsRecording() {
		t.Fatal("expected daemon to stop recording after hotkey release")
	}

	// 4. Verify text was post-processed and pasted
	expectedText := "Cadence speech to text!"
	if paster.pastedText != expectedText {
		t.Fatalf("expected pasted text %q, got %q", expectedText, paster.pastedText)
	}

	// 5. Verify transcript persistence
	history, err := store.ListTranscripts(10)
	if err != nil {
		t.Fatalf("ListTranscripts failed: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 transcript stored, got %d", len(history))
	}
	if history[0].Text != expectedText {
		t.Fatalf("expected stored transcript %q, got %q", expectedText, history[0].Text)
	}
}

func WithCapture(c audio.Capture) daemon.Option { return daemon.WithCapture(c) }
func WithASR(e asr.Engine) daemon.Option        { return daemon.WithASR(e) }
func WithPaster(p paste.Paster) daemon.Option   { return daemon.WithPaster(p) }
func WithStore(s storage.Store) daemon.Option   { return daemon.WithStore(s) }
func WithPostprocess(p *postprocess.Pipeline) daemon.Option {
	return daemon.WithPostprocess(p)
}
func WithHotkeyListener(l hotkey.Listener) daemon.Option { return daemon.WithHotkeyListener(l) }
func WithIPCListener(l net.Listener) daemon.Option       { return daemon.WithIPCListener(l) }
