package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/config"
	"github.com/AdeshDeshmukh/cadence/internal/hotkey"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/pipeline"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
	"github.com/AdeshDeshmukh/cadence/internal/vad"
	"github.com/AdeshDeshmukh/cadence/internal/worker"
	"github.com/AdeshDeshmukh/cadence/pkg/version"
)

// App manages the lifecycle, hardware bindings, and IPC for the Cadence daemon.
type App struct {
	cfg       config.Config
	kernel    *pipeline.Kernel
	capture   audio.Capture
	asr       asr.Engine
	workerMgr *worker.Manager
	paster    paste.Paster
	store     storage.Store
	process   *postprocess.Pipeline
	detector  vad.Detector
	hotkey    hotkey.Listener

	ipcServer    *ipc.Server
	ipcListener  net.Listener
	ipcSocket    string
	ownsListener bool

	mu            sync.Mutex
	isRecording   bool
	stopRequested bool
	activeCancel  context.CancelFunc
	activeSess    *pipeline.Session
	sessionSeq    uint64

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Option customizes App dependencies for production or testing.
type Option func(*App)

// WithCapture sets a custom audio capture implementation.
func WithCapture(c audio.Capture) Option {
	return func(a *App) { a.capture = c }
}

// WithASR sets a custom ASR engine.
func WithASR(e asr.Engine) Option {
	return func(a *App) { a.asr = e }
}

// WithPaster sets a custom text insertion paster.
func WithPaster(p paste.Paster) Option {
	return func(a *App) { a.paster = p }
}

// WithStore sets a custom transcript store.
func WithStore(s storage.Store) Option {
	return func(a *App) { a.store = s }
}

// WithPostprocess sets a custom text post-processing pipeline.
func WithPostprocess(p *postprocess.Pipeline) Option {
	return func(a *App) { a.process = p }
}

// WithDetector sets a custom voice activity detector.
func WithDetector(d vad.Detector) Option {
	return func(a *App) { a.detector = d }
}

// WithHotkeyListener sets a custom hotkey listener.
func WithHotkeyListener(l hotkey.Listener) Option {
	return func(a *App) { a.hotkey = l }
}

// WithIPCListener sets a pre-bound network listener for the IPC server.
func WithIPCListener(l net.Listener) Option {
	return func(a *App) {
		a.ipcListener = l
		a.ownsListener = false
	}
}

// WithWorkerManager sets an external Python worker manager.
func WithWorkerManager(m *worker.Manager) Option {
	return func(a *App) { a.workerMgr = m }
}

// NewApp instantiates a fully configured Cadence daemon application.
func NewApp(cfg config.Config, opts ...Option) *App {
	a := &App{
		cfg:          cfg,
		ownsListener: true,
	}

	for _, opt := range opts {
		opt(a)
	}

	if a.store == nil {
		if cfg.Storage.DBPath != "" {
			if s, err := storage.Open(cfg.Storage.DBPath); err == nil {
				a.store = s
			} else {
				a.store = storage.NewMemory()
			}
		} else {
			a.store = storage.NewMemory()
		}
	}

	if a.process == nil {
		var steps []postprocess.Step
		if cfg.Post.FillerRemoval {
			steps = append(steps, postprocess.NewFillerRemoval())
		}
		if cfg.Post.Vocabulary {
			steps = append(steps, postprocess.NewVocabulary())
		}
		if cfg.Post.ITN {
			steps = append(steps, postprocess.NewITN())
		}
		if cfg.Post.Punctuation {
			steps = append(steps, postprocess.NewPunctuation())
		}
		a.process = postprocess.New(steps...)
	}

	if a.detector == nil {
		if cfg.VAD.Engine == "silero" && a.workerMgr != nil {
			hangover := time.Duration(cfg.VAD.HangoverMs) * time.Millisecond
			if hangover <= 0 {
				hangover = 500 * time.Millisecond
			}
			thresh := cfg.VAD.Threshold
			if thresh <= 0 {
				thresh = 0.5
			}
			a.detector = vad.NewSileroDetector(a.workerMgr,
				vad.WithSileroThreshold(thresh),
				vad.WithSileroHangover(hangover),
			)
		} else {
			a.detector = vad.NewAdaptive()
		}
	}

	if a.paster == nil {
		a.paster = paste.NewDefaultCascade()
	}

	if a.capture == nil {
		a.capture = audio.NewEngine(
			audio.WithSampleRate(cfg.Audio.SampleRate),
		)
	}

	if a.asr == nil {
		if cfg.ASR.Engine == "whisper" && a.workerMgr != nil {
			a.asr = asr.NewWhisperEngine(a.workerMgr)
		} else {
			a.asr = asr.MockEngine{Text: "Cadence local dictation"}
		}
	}

	if a.kernel == nil {
		a.kernel = pipeline.NewKernel(
			a.capture, a.asr, a.paster, a.process, a.store,
		)
	}

	if a.hotkey == nil {
		mode := hotkey.ModePushToTalk
		if cfg.Hotkey.Mode == string(hotkey.ModeToggle) {
			mode = hotkey.ModeToggle
		}
		a.hotkey = hotkey.NewDefaultListener(cfg.Hotkey.Shortcut, mode)
	}

	return a
}

// Start launches the daemon's background tasks: IPC server, worker manager, and hotkey listener.
func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.ctx, a.cancel = context.WithCancel(ctx)

	if a.workerMgr != nil {
		if err := a.workerMgr.Start(a.ctx); err != nil {
			return fmt.Errorf("start worker manager: %w", err)
		}
	}

	if a.ipcListener == nil && a.cfg.IPC.SocketPath != "" {
		sockPath := a.cfg.IPC.SocketPath
		_ = os.Remove(sockPath)
		_ = os.MkdirAll(filepath.Dir(sockPath), 0755)

		l, err := net.Listen("unix", sockPath)
		if err != nil {
			return fmt.Errorf("listen ipc unix socket: %w", err)
		}
		a.ipcListener = l
		a.ipcSocket = sockPath
		a.ownsListener = true
	}

	if a.ipcListener != nil {
		a.ipcServer = ipc.NewServer(a.ipcListener, a)
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			_ = a.ipcServer.Serve(a.ctx)
		}()
		go func() {
			<-a.ctx.Done()
			_ = a.ipcListener.Close()
		}()
	}

	if a.hotkey != nil {
		events, err := a.hotkey.Start()
		if err != nil {
			return fmt.Errorf("start hotkey listener: %w", err)
		}

		a.wg.Add(1)
		go a.eventLoop(events)
	}

	return nil
}

func (a *App) eventLoop(events <-chan hotkey.Event) {
	defer a.wg.Done()

	mode := hotkey.Mode(a.cfg.Hotkey.Mode)
	if mode == "" {
		mode = hotkey.ModePushToTalk
	}

	for {
		select {
		case <-a.ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			a.handleHotkeyEvent(ev, mode)
		}
	}
}

func (a *App) handleHotkeyEvent(ev hotkey.Event, mode hotkey.Mode) {
	switch mode {
	case hotkey.ModePushToTalk:
		if ev.Type == hotkey.EventKeyDown {
			a.startDictation()
		} else if ev.Type == hotkey.EventKeyUp {
			a.stopDictation()
		}
	case hotkey.ModeToggle:
		if ev.Type == hotkey.EventKeyDown {
			a.mu.Lock()
			recording := a.isRecording
			a.mu.Unlock()

			if !recording {
				a.startDictation()
			} else {
				a.stopDictation()
			}
		}
	}
}

func (a *App) startDictation() {
	a.mu.Lock()
	if a.isRecording {
		a.mu.Unlock()
		return
	}

	a.isRecording = true
	a.stopRequested = false
	sessID := fmt.Sprintf("sess-%d", atomic.AddUint64(&a.sessionSeq, 1))
	sess := pipeline.NewSession(sessID)
	a.activeSess = sess

	sessCtx, cancel := context.WithCancel(a.ctx)
	a.activeCancel = cancel
	a.mu.Unlock()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer func() {
			a.mu.Lock()
			a.isRecording = false
			a.stopRequested = false
			a.activeCancel = nil
			a.activeSess = nil
			a.mu.Unlock()
		}()

		stopWatcher := make(chan struct{})
		defer close(stopWatcher)
		go func() {
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stopWatcher:
					return
				case <-sessCtx.Done():
					return
				case <-ticker.C:
					a.mu.Lock()
					stopped := a.stopRequested
					a.mu.Unlock()
					if stopped {
						_ = a.capture.Stop()
						return
					}
				}
			}
		}()

		pc := postprocess.Context{
			Mode: "dictation",
		}
		_ = a.kernel.Run(sessCtx, sess, pc)
	}()
}

func (a *App) stopDictation() {
	a.mu.Lock()
	if !a.isRecording {
		a.mu.Unlock()
		return
	}
	a.stopRequested = true
	a.mu.Unlock()

	_ = a.capture.Stop()
}

// IsRecording returns true if an audio dictation session is actively recording.
func (a *App) IsRecording() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.isRecording
}

// ActiveSession returns the current active dictation session, if any.
func (a *App) ActiveSession() *pipeline.Session {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.activeSess
}

// Detector returns the active voice activity detector.
func (a *App) Detector() vad.Detector {
	return a.detector
}

// Handle routes incoming IPC client commands.
func (a *App) Handle(_ context.Context, req ipc.Request) (any, error) {
	switch req.Method {
	case "ping":
		return "pong", nil
	case "status":
		a.mu.Lock()
		rec := a.isRecording
		var sessID string
		var partial string
		if a.activeSess != nil {
			sessID = a.activeSess.ID
			partial = a.activeSess.GetPartial()
		}
		a.mu.Unlock()

		return map[string]any{
			"status":    "running",
			"version":   version.Version,
			"recording": rec,
			"session":   sessID,
			"partial":   partial,
			"shortcut":  a.cfg.Hotkey.Shortcut,
			"mode":      a.cfg.Hotkey.Mode,
		}, nil
	case "dictate_start":
		a.startDictation()
		return map[string]any{"ok": true, "action": "started"}, nil
	case "dictate_stop":
		a.stopDictation()
		return map[string]any{"ok": true, "action": "stopped"}, nil
	case "history":
		limit := 20
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if limVal, ok := params["limit"].(float64); ok && limVal > 0 {
				limit = int(limVal)
			}
		}
		return a.store.ListTranscripts(limit)
	case "recover":
		rec, err := a.store.GetLastRecovery()
		if err != nil {
			return nil, fmt.Errorf("get last recovery: %w", err)
		}
		if rec == nil {
			return nil, nil
		}
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if shouldPaste, _ := params["paste"].(bool); shouldPaste {
				best := rec.BestText()
				if best != "" && a.paster != nil {
					_, _ = a.paster.Paste(best)
				}
			}
		}
		return rec, nil
	case "recoveries":
		limit := 20
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if limVal, ok := params["limit"].(float64); ok && limVal > 0 {
				limit = int(limVal)
			}
		}
		return a.store.ListRecoveries(limit)
	case "vocabulary":
		return a.store.Vocabulary()
	default:
		return nil, errors.New("unknown method: " + req.Method)
	}
}

// Run starts the daemon and blocks until the provided context is cancelled.
func (a *App) Run(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		return err
	}

	<-a.ctx.Done()
	return a.Stop()
}

// Stop terminates all daemon subsystems, releases hardware locks, and cleans up IPC sockets.
func (a *App) Stop() error {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}

	if a.activeCancel != nil {
		a.activeCancel()
	}
	if a.isRecording {
		_ = a.capture.Stop()
		a.isRecording = false
	}
	a.mu.Unlock()

	if a.hotkey != nil {
		_ = a.hotkey.Stop()
	}

	if a.ipcListener != nil {
		_ = a.ipcListener.Close()
		if a.ipcSocket != "" && a.ownsListener {
			_ = os.Remove(a.ipcSocket)
		}
	}

	if a.workerMgr != nil {
		_ = a.workerMgr.Stop()
	}

	if a.store != nil {
		if c, ok := a.store.(io.Closer); ok {
			_ = c.Close()
		}
	}

	a.wg.Wait()
	return nil
}
