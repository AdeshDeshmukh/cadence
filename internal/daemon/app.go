package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/AdeshDeshmukh/cadence/internal/polish"
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
	polisher  polish.Polisher
	profiles  *polish.ProfileRegistry
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

// WithPolisher sets a custom LLM text polisher.
func WithPolisher(p polish.Polisher) Option {
	return func(a *App) { a.polisher = p }
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

	if a.profiles == nil {
		a.profiles = polish.NewProfileRegistry(a.store)
	}

	if a.polisher == nil && cfg.Polish.Enabled {
		switch cfg.Polish.Backend {
		case "ollama":
			endpoint := cfg.Polish.Endpoint
			if endpoint == "" {
				endpoint = "http://127.0.0.1:11434"
			}
			model := cfg.Polish.Model
			if model == "" {
				model = "llama3.2"
			}
			a.polisher = polish.NewOllamaPolisher(
				polish.WithOllamaEndpoint(endpoint),
				polish.WithOllamaModel(model),
				polish.WithOllamaProfiles(a.profiles),
			)
		case "openai":
			endpoint := cfg.Polish.Endpoint
			if endpoint == "" {
				endpoint = "https://api.openai.com/v1"
			}
			model := cfg.Polish.Model
			if model == "" {
				model = "gpt-4o-mini"
			}
			a.polisher = polish.NewOpenAIPolisher(
				polish.WithOpenAIEndpoint(endpoint),
				polish.WithOpenAIModel(model),
				polish.WithOpenAIAPIKey(cfg.Polish.APIKey),
				polish.WithOpenAINetworkMode(cfg.Privacy.NetworkMode),
				polish.WithOpenAIProfiles(a.profiles),
			)
		default:
			a.polisher = polish.NewNoopPolisher()
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
		if cfg.Post.Snippets {
			steps = append(steps, postprocess.NewSnippets())
		}
		if cfg.Polish.Enabled && a.polisher != nil {
			timeout := time.Duration(cfg.Polish.TimeoutMs) * time.Millisecond
			if timeout <= 0 {
				timeout = 2 * time.Second
			}
			steps = append(steps, polish.NewStep(a.polisher, cfg.Polish.DefaultProfile, timeout))
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
		basePaster := paste.NewDefaultCascade()
		delay := time.Duration(cfg.Paste.RestoreDelayMs) * time.Millisecond
		if delay <= 0 {
			delay = 80 * time.Millisecond
		}
		a.paster = paste.NewPreservingPaster(basePaster, cfg.Paste.PreserveClipboard, delay)
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

		vocabMap := make(map[string]string)
		snippetMap := make(map[string]string)
		if a.store != nil {
			if entries, err := a.store.Vocabulary(); err == nil {
				for _, e := range entries {
					vocabMap[e.Spoken] = e.Canonical
				}
			}
			if snips, err := a.store.ListSnippets(); err == nil {
				for _, s := range snips {
					snippetMap[s.Trigger] = s.Template
				}
			}
		}

		pc := postprocess.Context{
			Mode:       "dictation",
			Vocabulary: vocabMap,
			Snippets:   snippetMap,
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

// Polisher returns the active LLM text polisher.
func (a *App) Polisher() polish.Polisher {
	return a.polisher
}

// Profiles returns the active prompt profile registry.
func (a *App) Profiles() *polish.ProfileRegistry {
	return a.profiles
}

// Handle routes incoming IPC client commands.
func (a *App) Handle(ctx context.Context, req ipc.Request) (any, error) {
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
			"status":         "running",
			"version":        version.Version,
			"recording":      rec,
			"session":        sessID,
			"partial":        partial,
			"shortcut":       a.cfg.Hotkey.Shortcut,
			"mode":           a.cfg.Hotkey.Mode,
			"polish_enabled": a.cfg.Polish.Enabled,
			"polish_backend": a.cfg.Polish.Backend,
			"polish_model":   a.cfg.Polish.Model,
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
	case "vocabulary", "vocab_list":
		return a.store.Vocabulary()
	case "vocab_add":
		params, ok := req.Params.(map[string]any)
		if !ok || params == nil {
			return nil, errors.New("missing parameters for vocab_add")
		}
		spoken, _ := params["spoken"].(string)
		canonical, _ := params["canonical"].(string)
		if strings.TrimSpace(spoken) == "" || strings.TrimSpace(canonical) == "" {
			return nil, errors.New("spoken and canonical terms must not be empty")
		}
		if err := a.store.SetVocabulary(storage.VocabularyEntry{
			Spoken:    spoken,
			Canonical: canonical,
		}); err != nil {
			return nil, fmt.Errorf("set vocabulary: %w", err)
		}
		return map[string]any{"ok": true, "spoken": spoken, "canonical": canonical}, nil

	case "vocab_remove", "vocab_delete":
		params, ok := req.Params.(map[string]any)
		if !ok || params == nil {
			return nil, errors.New("missing parameters for vocab_remove")
		}
		spoken, _ := params["spoken"].(string)
		if strings.TrimSpace(spoken) == "" {
			return nil, errors.New("spoken term must not be empty")
		}
		if err := a.store.DeleteVocabulary(spoken); err != nil {
			return nil, fmt.Errorf("delete vocabulary: %w", err)
		}
		return map[string]any{"ok": true, "spoken": spoken}, nil

	case "vocab_import":
		params, ok := req.Params.(map[string]any)
		if !ok || params == nil {
			return nil, errors.New("missing parameters for vocab_import")
		}
		content, _ := params["content"].(string)
		format, _ := params["format"].(string)
		count, err := storage.ImportVocabulary(a.store, strings.NewReader(content), format)
		if err != nil {
			return nil, fmt.Errorf("import vocabulary: %w", err)
		}
		return map[string]any{"ok": true, "count": count}, nil

	case "vocab_export":
		format := "json"
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if f, ok := params["format"].(string); ok && f != "" {
				format = f
			}
		}
		var buf bytes.Buffer
		if err := storage.ExportVocabulary(a.store, format, &buf); err != nil {
			return nil, fmt.Errorf("export vocabulary: %w", err)
		}
		return map[string]any{"ok": true, "format": format, "content": buf.String()}, nil

	case "snippet_list":
		return a.store.ListSnippets()

	case "snippet_set":
		params, ok := req.Params.(map[string]any)
		if !ok || params == nil {
			return nil, errors.New("missing parameters for snippet_set")
		}
		trigger, _ := params["trigger"].(string)
		template, _ := params["template"].(string)
		desc, _ := params["description"].(string)
		if strings.TrimSpace(trigger) == "" || strings.TrimSpace(template) == "" {
			return nil, errors.New("trigger and template cannot be empty")
		}
		snip := storage.Snippet{
			Trigger:     trigger,
			Template:    template,
			Description: desc,
		}
		if err := a.store.SetSnippet(snip); err != nil {
			return nil, fmt.Errorf("set snippet: %w", err)
		}
		return map[string]any{"ok": true, "trigger": trigger}, nil

	case "snippet_get":
		params, ok := req.Params.(map[string]any)
		if !ok || params == nil {
			return nil, errors.New("missing parameters for snippet_get")
		}
		trigger, _ := params["trigger"].(string)
		return a.store.GetSnippet(trigger)

	case "snippet_delete":
		params, ok := req.Params.(map[string]any)
		if !ok || params == nil {
			return nil, errors.New("missing parameters for snippet_delete")
		}
		trigger, _ := params["trigger"].(string)
		if err := a.store.DeleteSnippet(trigger); err != nil {
			return nil, fmt.Errorf("delete snippet: %w", err)
		}
		return map[string]any{"ok": true, "trigger": trigger}, nil

	case "snippet_expand":
		params, ok := req.Params.(map[string]any)
		if !ok || params == nil {
			return nil, errors.New("missing parameters for snippet_expand")
		}
		template, _ := params["template"].(string)
		if template == "" {
			trigger, _ := params["trigger"].(string)
			if trigger != "" {
				s, err := a.store.GetSnippet(trigger)
				if err != nil {
					return nil, err
				}
				if s != nil {
					template = s.Template
				}
			}
		}
		clip, _ := paste.GetSystemClipboard()
		expanded := postprocess.ExpandVariables(template, clip, time.Now())
		return map[string]any{"ok": true, "template": template, "expanded": expanded}, nil

	case "polish":
		params, _ := req.Params.(map[string]any)
		text, _ := params["text"].(string)
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("missing or empty text parameter for polish")
		}
		profile, _ := params["profile"].(string)
		if profile == "" {
			profile = a.cfg.Polish.DefaultProfile
		}
		appContext, _ := params["application"].(string)

		if a.polisher == nil {
			return map[string]any{
				"text":       text,
				"model":      "none",
				"latency_ms": 0,
			}, nil
		}

		timeout := time.Duration(a.cfg.Polish.TimeoutMs) * time.Millisecond
		if timeout <= 0 {
			timeout = 2 * time.Second
		}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		resp, err := a.polisher.Polish(callCtx, polish.Request{
			Text:        text,
			Profile:     profile,
			Application: appContext,
		})
		if err != nil {
			return nil, fmt.Errorf("polish: %w", err)
		}
		return map[string]any{
			"text":       resp.Text,
			"model":      resp.Model,
			"tokens_in":  resp.TokensIn,
			"tokens_out": resp.TokensOut,
			"latency_ms": resp.Latency.Milliseconds(),
		}, nil

	case "polish_profile_list":
		if a.profiles == nil {
			return polish.Builtins(), nil
		}
		return a.profiles.List(), nil

	case "polish_profile_get":
		params, _ := req.Params.(map[string]any)
		name, _ := params["name"].(string)
		if name == "" {
			return nil, errors.New("missing name parameter")
		}
		if a.profiles == nil {
			return nil, fmt.Errorf("profile %q not found", name)
		}
		p, ok := a.profiles.Get(name)
		if !ok {
			return nil, fmt.Errorf("profile %q not found", name)
		}
		return p, nil

	case "polish_profile_set":
		params, _ := req.Params.(map[string]any)
		name, _ := params["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("missing name parameter")
		}
		desc, _ := params["description"].(string)
		prompt, _ := params["system_prompt"].(string)
		temp, _ := params["temperature"].(float64)
		var rules []string
		if rawRules, ok := params["custom_rules"].([]any); ok {
			for _, r := range rawRules {
				if s, ok := r.(string); ok && strings.TrimSpace(s) != "" {
					rules = append(rules, s)
				}
			}
		}
		if a.profiles == nil {
			return nil, errors.New("no profile registry configured")
		}
		p := polish.Profile{
			Name:         name,
			Description:  desc,
			SystemPrompt: prompt,
			Temperature:  temp,
			CustomRules:  rules,
		}
		if err := a.profiles.Set(p); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "name": name}, nil

	case "polish_profile_delete":
		params, _ := req.Params.(map[string]any)
		name, _ := params["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("missing name parameter")
		}
		if a.profiles == nil {
			return nil, errors.New("no profile registry configured")
		}
		if err := a.profiles.Delete(name); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "name": name}, nil

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
