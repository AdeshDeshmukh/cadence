package daemon

import (
	"bytes"
	"context"
	"encoding/json"
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
	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
	"github.com/AdeshDeshmukh/cadence/internal/developer"
	"github.com/AdeshDeshmukh/cadence/internal/edit"
	"github.com/AdeshDeshmukh/cadence/internal/hotkey"
	"github.com/AdeshDeshmukh/cadence/internal/ipc"
	"github.com/AdeshDeshmukh/cadence/internal/paste"
	"github.com/AdeshDeshmukh/cadence/internal/pipeline"
	"github.com/AdeshDeshmukh/cadence/internal/polish"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
	"github.com/AdeshDeshmukh/cadence/internal/selection"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
	"github.com/AdeshDeshmukh/cadence/internal/vad"
	"github.com/AdeshDeshmukh/cadence/internal/worker"
	"github.com/AdeshDeshmukh/cadence/pkg/version"
)

// UndoRecord captures the transformation details of a completed dictation session.
type UndoRecord struct {
	SessionID      string    `json:"session_id"`
	RawText        string    `json:"raw_text"`
	UnpolishedText string    `json:"unpolished_text"`
	PolishedText   string    `json:"polished_text"`
	IsPolished     bool      `json:"is_polished"`
	PastedAt       time.Time `json:"pasted_at"`
	Undone         bool      `json:"undone"`
}

// UndoResult describes the outcome of an undo operation.
type UndoResult struct {
	SessionID    string `json:"session_id"`
	RevertedFrom string `json:"reverted_from"`
	RevertedTo   string `json:"reverted_to"`
	Target       string `json:"target"`
	Remaining    int    `json:"remaining"`
}

// App manages the lifecycle, hardware bindings, and IPC for the Cadence daemon.
type App struct {
	cfg         config.Config
	kernel      *pipeline.Kernel
	capture     audio.Capture
	asr         asr.Engine
	workerMgr   *worker.Manager
	paster      paste.Paster
	reverter    paste.Reverter
	capturer    selection.Capturer
	editParser  *edit.Parser
	editor      *edit.Editor
	store       storage.Store
	process     *postprocess.Pipeline
	corrector   *postprocess.Corrector
	polisher    polish.Polisher
	profiles    *polish.ProfileRegistry
	router      *polish.Router
	appDetector appcontext.Detector
	detector    vad.Detector
	hotkey      hotkey.Listener

	ipcServer    *ipc.Server
	ipcListener  net.Listener
	ipcSocket    string
	ownsListener bool

	mu               sync.Mutex
	isRecording      bool
	stopRequested    bool
	activeCancel     context.CancelFunc
	activeSess       *pipeline.Session
	sessionSeq       uint64
	lastUndo         *UndoRecord
	undoStack        []*UndoRecord
	previewMgr       *edit.PreviewManager
	editorContext    *appcontext.EditorContextManager
	terminalContext  *appcontext.TerminalContextManager
	devGenerator     *developer.Generator
	devPreviewEngine *developer.PreviewEngine
	gitAssistant     *developer.GitAssistant

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

// WithAppDetector sets a custom active application context detector.
func WithAppDetector(d appcontext.Detector) Option {
	return func(a *App) { a.appDetector = d }
}

// WithRouter sets a custom polish application route manager.
func WithRouter(r *polish.Router) Option {
	return func(a *App) { a.router = r }
}

// WithReverter sets a custom text reverter implementation.
func WithReverter(r paste.Reverter) Option {
	return func(a *App) { a.reverter = r }
}

// WithCorrector sets a custom ASR self-corrector.
func WithCorrector(c *postprocess.Corrector) Option {
	return func(a *App) { a.corrector = c }
}

// WithSelectionCapturer sets a custom selection capturer implementation.
func WithSelectionCapturer(c selection.Capturer) Option {
	return func(a *App) { a.capturer = c }
}

// WithEditParser sets a custom voice instruction edit parser.
func WithEditParser(p *edit.Parser) Option {
	return func(a *App) { a.editParser = p }
}

// WithEditor sets a custom text editor implementation.
func WithEditor(e *edit.Editor) Option {
	return func(a *App) { a.editor = e }
}

// WithPreviewManager sets a custom preview session manager.
func WithPreviewManager(m *edit.PreviewManager) Option {
	return func(a *App) { a.previewMgr = m }
}

// WithEditorContextManager sets a custom editor context manager.
func WithEditorContextManager(m *appcontext.EditorContextManager) Option {
	return func(a *App) { a.editorContext = m }
}

// WithTerminalContextManager sets a custom terminal context manager.
func WithTerminalContextManager(m *appcontext.TerminalContextManager) Option {
	return func(a *App) { a.terminalContext = m }
}

// WithDevGenerator sets a custom developer command generator.
func WithDevGenerator(g *developer.Generator) Option {
	return func(a *App) { a.devGenerator = g }
}

// WithDevPreviewEngine sets a custom developer dry-run preview and execution engine.
func WithDevPreviewEngine(e *developer.PreviewEngine) Option {
	return func(a *App) { a.devPreviewEngine = e }
}

// WithGitAssistant sets a custom Git assistant.
func WithGitAssistant(g *developer.GitAssistant) Option {
	return func(a *App) { a.gitAssistant = g }
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

	if a.appDetector == nil {
		a.appDetector = appcontext.NewDefaultDetector()
	}

	if a.router == nil {
		a.router = polish.NewRouter(cfg.Polish.AppRoutes)
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

	if a.corrector == nil {
		a.corrector = postprocess.NewCorrector(cfg.Post.Corrections)
	}

	if a.process == nil {
		var steps []postprocess.Step
		if cfg.Post.FillerRemoval {
			steps = append(steps, postprocess.NewFillerRemoval())
		}
		if cfg.Post.Correction && a.corrector != nil {
			steps = append(steps, a.corrector)
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
			step := polish.NewStep(a.polisher, cfg.Polish.DefaultProfile, timeout)
			if cfg.Polish.AutoRoute && a.router != nil {
				step = step.WithRouter(a.router)
			}
			steps = append(steps, step)
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

	if a.reverter == nil {
		a.reverter = paste.NewNativeReverter()
	}

	if a.capturer == nil {
		a.capturer = selection.NewDefaultCapturer()
	}

	if a.editParser == nil {
		a.editParser = edit.NewParser(a.polisher)
	}

	if a.editor == nil {
		a.editor = edit.NewEditor(a.polisher)
	}

	if a.previewMgr == nil {
		a.previewMgr = edit.NewPreviewManager(50)
	}

	if a.editorContext == nil {
		a.editorContext = appcontext.NewEditorContextManager()
	}

	if a.terminalContext == nil {
		a.terminalContext = appcontext.NewTerminalContextManager()
	}

	if a.devGenerator == nil {
		a.devGenerator = developer.NewGenerator(a.polisher)
	}

	if a.devPreviewEngine == nil {
		a.devPreviewEngine = developer.NewPreviewEngine()
	}

	if a.gitAssistant == nil {
		a.gitAssistant = developer.NewGitAssistant(a.polisher)
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
	if a.cfg.Hotkey.UndoShortcut != "" && strings.EqualFold(ev.Shortcut, a.cfg.Hotkey.UndoShortcut) {
		if ev.Type == hotkey.EventKeyDown {
			_, _ = a.UndoTransformation("unpolished")
		}
		return
	}

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

		if a.appDetector != nil {
			go func() {
				detectCtx, cancel := context.WithTimeout(sessCtx, 250*time.Millisecond)
				defer cancel()
				if appInfo, err := a.appDetector.Detect(detectCtx); err == nil && appInfo.Name != "" {
					sess.SetApplication(appInfo.Name)
				}
			}()
		}

		pc := postprocess.Context{
			Mode:       "dictation",
			Vocabulary: vocabMap,
			Snippets:   snippetMap,
		}
		_ = a.kernel.Run(sessCtx, sess, pc)

		if sess.FinalText != "" && sess.GetState() == pipeline.Complete {
			a.mu.Lock()
			unpolished := sess.GetUnpolishedText()
			if unpolished == "" {
				unpolished = sess.FinalText
			}
			a.recordUndoLocked(&UndoRecord{
				SessionID:      sess.ID,
				RawText:        sess.RawText,
				UnpolishedText: unpolished,
				PolishedText:   sess.FinalText,
				IsPolished:     unpolished != sess.FinalText,
				PastedAt:       time.Now(),
				Undone:         false,
			})
			a.mu.Unlock()
		}
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

// AppDetector returns the active application context detector.
func (a *App) AppDetector() appcontext.Detector {
	return a.appDetector
}

// EditorContextManager returns the active code editor context manager.
func (a *App) EditorContextManager() *appcontext.EditorContextManager {
	return a.editorContext
}

// TerminalContextManager returns the active terminal context manager.
func (a *App) TerminalContextManager() *appcontext.TerminalContextManager {
	return a.terminalContext
}

// DevGenerator returns the active developer command generator.
func (a *App) DevGenerator() *developer.Generator {
	return a.devGenerator
}

// DevPreviewEngine returns the active developer preview and execution engine.
func (a *App) DevPreviewEngine() *developer.PreviewEngine {
	return a.devPreviewEngine
}

// GitAssistant returns the active Git assistant.
func (a *App) GitAssistant() *developer.GitAssistant {
	return a.gitAssistant
}

// Router returns the active application context router.
func (a *App) Router() *polish.Router {
	return a.router
}

// Reverter returns the active text paste reverter.
func (a *App) Reverter() paste.Reverter {
	return a.reverter
}

// Corrector returns the active ASR self-correction engine.
func (a *App) Corrector() *postprocess.Corrector {
	return a.corrector
}

// SelectionCapturer returns the active selection capturer.
func (a *App) SelectionCapturer() selection.Capturer {
	return a.capturer
}

// EditParser returns the active voice instruction parser.
func (a *App) EditParser() *edit.Parser {
	return a.editParser
}

// Editor returns the active voice text editor.
func (a *App) Editor() *edit.Editor {
	return a.editor
}

func (a *App) recordUndoLocked(rec *UndoRecord) {
	a.lastUndo = rec
	a.undoStack = append(a.undoStack, rec)
	if len(a.undoStack) > 50 {
		a.undoStack = a.undoStack[1:]
	}
}

// CanUndo reports whether there is an AI transformation available to revert in the undo stack.
func (a *App) CanUndo() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := len(a.undoStack) - 1; i >= 0; i-- {
		if !a.undoStack[i].Undone && a.undoStack[i].IsPolished {
			return true
		}
	}
	return a.lastUndo != nil && !a.lastUndo.Undone && a.lastUndo.IsPolished
}

// LastUndo returns a snapshot of the top reversible record from the undo stack.
func (a *App) LastUndo() *UndoRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := len(a.undoStack) - 1; i >= 0; i-- {
		if !a.undoStack[i].Undone && a.undoStack[i].IsPolished {
			cpy := *a.undoStack[i]
			return &cpy
		}
	}
	if a.lastUndo == nil {
		return nil
	}
	cpy := *a.lastUndo
	return &cpy
}

// UndoStack returns a snapshot of the full undo history stack.
func (a *App) UndoStack() []*UndoRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]*UndoRecord, len(a.undoStack))
	for i, r := range a.undoStack {
		cpy := *r
		out[i] = &cpy
	}
	return out
}

// PreviewManager returns the active preview session manager.
func (a *App) PreviewManager() *edit.PreviewManager {
	return a.previewMgr
}

// UndoTransformation reverts the most recent AI-polished or edited text back to original/raw text.
func (a *App) UndoTransformation(target string) (*UndoResult, error) {
	a.mu.Lock()
	var rec *UndoRecord
	for i := len(a.undoStack) - 1; i >= 0; i-- {
		if !a.undoStack[i].Undone && a.undoStack[i].IsPolished {
			rec = a.undoStack[i]
			break
		}
	}
	if rec == nil {
		rec = a.lastUndo
	}
	if rec == nil {
		a.mu.Unlock()
		return nil, errors.New("no dictation or edit session available to undo")
	}
	if rec.Undone {
		a.mu.Unlock()
		return nil, errors.New("last transformation has already been undone")
	}

	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		target = "unpolished"
	}

	if target != "raw" && !rec.IsPolished {
		a.mu.Unlock()
		return nil, errors.New("no AI transformation to undo: polished text matches unpolished text")
	}

	replacementText := rec.UnpolishedText
	if target == "raw" {
		replacementText = rec.RawText
	}
	a.mu.Unlock()

	// Revert the paste in the focused UI element
	if a.reverter != nil {
		if err := a.reverter.Revert(); err != nil {
			return nil, fmt.Errorf("revert previous paste: %w", err)
		}
	}

	// Brief pause to allow the focused application to process the undo action
	time.Sleep(50 * time.Millisecond)

	// Paste the replacement text
	if a.paster != nil && replacementText != "" {
		if _, err := a.paster.Paste(replacementText); err != nil {
			return nil, fmt.Errorf("paste replacement text: %w", err)
		}
	}

	a.mu.Lock()
	rec.Undone = true

	// Count remaining reversible items in the undo stack
	remaining := 0
	for _, r := range a.undoStack {
		if !r.Undone && r.IsPolished {
			remaining++
		}
	}

	res := &UndoResult{
		SessionID:    rec.SessionID,
		RevertedFrom: rec.PolishedText,
		RevertedTo:   replacementText,
		Target:       target,
		Remaining:    remaining,
	}
	a.mu.Unlock()

	return res, nil
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

		var appName string
		if a.appDetector != nil {
			if info, err := a.appDetector.Detect(ctx); err == nil && info.Name != "" {
				appName = info.Name
			}
		}

		canUndo := a.CanUndo()
		var lastUndoMap map[string]any
		if rec := a.LastUndo(); rec != nil {
			lastUndoMap = map[string]any{
				"session_id":  rec.SessionID,
				"is_polished": rec.IsPolished,
				"undone":      rec.Undone,
				"pasted_at":   rec.PastedAt,
			}
		}

		return map[string]any{
			"status":             "running",
			"version":            version.Version,
			"recording":          rec,
			"session":            sessID,
			"partial":            partial,
			"shortcut":           a.cfg.Hotkey.Shortcut,
			"undo_shortcut":      a.cfg.Hotkey.UndoShortcut,
			"mode":               a.cfg.Hotkey.Mode,
			"polish_enabled":     a.cfg.Polish.Enabled,
			"polish_backend":     a.cfg.Polish.Backend,
			"polish_model":       a.cfg.Polish.Model,
			"active_app":         appName,
			"can_undo":           canUndo,
			"last_undo":          lastUndoMap,
			"correction_enabled": a.cfg.Post.Correction,
		}, nil

	case "undo":
		target := "unpolished"
		depth := 1
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if t, ok := params["target"].(string); ok && t != "" {
				target = t
			}
			if d, ok := params["depth"].(float64); ok && d > 0 {
				depth = int(d)
			}
		}
		var lastRes *UndoResult
		for i := 0; i < depth; i++ {
			if !a.CanUndo() {
				if i == 0 {
					return nil, errors.New("no dictation or edit session available to undo")
				}
				break
			}
			res, err := a.UndoTransformation(target)
			if err != nil {
				if i == 0 {
					return nil, err
				}
				break
			}
			lastRes = res
		}
		return lastRes, nil

	case "undo_list":
		return a.UndoStack(), nil

	case "active_app":
		if a.appDetector == nil {
			return appcontext.AppInfo{Name: "Unknown", Category: appcontext.CategoryGeneral}, nil
		}
		info, err := a.appDetector.Detect(ctx)
		if err != nil {
			return appcontext.AppInfo{Name: "Unknown", Category: appcontext.CategoryGeneral}, nil
		}
		return info, nil

	case "correct":
		params, _ := req.Params.(map[string]any)
		text, _ := params["text"].(string)
		app, _ := params["app"].(string)
		if app == "" && a.appDetector != nil {
			if info, err := a.appDetector.Detect(ctx); err == nil {
				app = info.Name
			}
		}
		corrector := a.corrector
		if corrector == nil {
			corrector = postprocess.NewCorrector(a.cfg.Post.Corrections)
		}
		report := corrector.Correct(text, app)
		return map[string]any{
			"original":            report.Original,
			"corrected":           report.Corrected,
			"app":                 app,
			"category":            string(report.Category),
			"changed":             report.Original != report.Corrected,
			"corrections_applied": report.CorrectionsApplied,
		}, nil

	case "editor_context_set":
		var eCtx appcontext.EditorContext
		raw, err := json.Marshal(req.Params)
		if err != nil {
			return nil, fmt.Errorf("invalid editor context params: %w", err)
		}
		if err := json.Unmarshal(raw, &eCtx); err != nil {
			return nil, fmt.Errorf("decode editor context: %w", err)
		}
		if a.editorContext != nil {
			a.editorContext.Set(eCtx)
		}
		return map[string]any{
			"status":     "ok",
			"updated_at": eCtx.UpdatedAt,
		}, nil

	case "editor_context_get":
		if a.editorContext == nil {
			return map[string]any{"active": false}, nil
		}
		ctxData, ok := a.editorContext.Get()
		if !ok {
			return map[string]any{"active": false}, nil
		}
		return map[string]any{
			"active":              true,
			"editor_name":         ctxData.EditorName,
			"workspace_root":      ctxData.WorkspaceRoot,
			"file_path":           ctxData.FilePath,
			"file_name":           ctxData.FileName,
			"language_id":         ctxData.LanguageID,
			"line":                ctxData.Line,
			"column":              ctxData.Column,
			"selection_text":      ctxData.SelectionText,
			"selection_range":     ctxData.SelectionRange,
			"surrounding_context": ctxData.SurroundingContext,
			"updated_at":          ctxData.UpdatedAt,
			"summary":             a.editorContext.FormatContextSummary(),
		}, nil

	case "editor_context_clear":
		if a.editorContext != nil {
			a.editorContext.Clear()
		}
		return map[string]any{"status": "cleared"}, nil

	case "terminal_context_set":
		var tCtx appcontext.TerminalContext
		raw, err := json.Marshal(req.Params)
		if err != nil {
			return nil, fmt.Errorf("invalid terminal context params: %w", err)
		}
		if err := json.Unmarshal(raw, &tCtx); err != nil {
			return nil, fmt.Errorf("decode terminal context: %w", err)
		}
		if a.terminalContext != nil {
			a.terminalContext.Set(tCtx)
		}
		return map[string]any{
			"status":     "ok",
			"updated_at": tCtx.UpdatedAt,
		}, nil

	case "terminal_context_get":
		if a.terminalContext == nil {
			return map[string]any{"active": false}, nil
		}
		tCtx, ok := a.terminalContext.Get()
		if !ok {
			return map[string]any{"active": false}, nil
		}
		return map[string]any{
			"active":          true,
			"terminal_app":    tCtx.TerminalApp,
			"shell":           tCtx.Shell,
			"pid":             tCtx.PID,
			"working_dir":     tCtx.WorkingDir,
			"git_branch":      tCtx.GitBranch,
			"last_command":    tCtx.LastCommand,
			"last_exit_code":  tCtx.LastExitCode,
			"recent_commands": tCtx.RecentCommands,
			"updated_at":      tCtx.UpdatedAt,
			"summary":         a.terminalContext.FormatContextSummary(),
		}, nil

	case "terminal_context_clear":
		if a.terminalContext != nil {
			a.terminalContext.Clear()
		}
		return map[string]any{"status": "cleared"}, nil

	case "dev_command_generate":
		if a.devGenerator == nil {
			return nil, errors.New("dev command generator is not configured")
		}
		var instruction, workingDir string
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if ins, ok := params["instruction"].(string); ok {
				instruction = ins
			}
			if wd, ok := params["working_dir"].(string); ok {
				workingDir = wd
			}
		}
		if strings.TrimSpace(instruction) == "" {
			return nil, errors.New("missing 'instruction' parameter for dev_command_generate")
		}
		if workingDir == "" && a.terminalContext != nil {
			if tCtx, ok := a.terminalContext.Get(); ok && tCtx.WorkingDir != "" {
				workingDir = tCtx.WorkingDir
			}
		}
		if workingDir == "" && a.editorContext != nil {
			if eCtx, ok := a.editorContext.Get(); ok && eCtx.WorkspaceRoot != "" {
				workingDir = eCtx.WorkspaceRoot
			}
		}
		cmdRes, err := a.devGenerator.Generate(ctx, instruction, workingDir)
		if err != nil {
			return nil, fmt.Errorf("generate dev command: %w", err)
		}
		return cmdRes, nil

	case "dev_command_risk":
		var command string
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if cmd, ok := params["command"].(string); ok {
				command = cmd
			}
		}
		if strings.TrimSpace(command) == "" {
			return nil, errors.New("missing 'command' parameter for dev_command_risk")
		}
		risk := developer.ClassifyCommand(command)
		return risk, nil

	case "dev_command_preview":
		if a.devPreviewEngine == nil {
			return nil, errors.New("dev preview engine is not configured")
		}
		var command, workingDir string
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if c, ok := params["command"].(string); ok {
				command = c
			}
			if wd, ok := params["working_dir"].(string); ok {
				workingDir = wd
			}
		}
		if strings.TrimSpace(command) == "" {
			return nil, errors.New("missing 'command' parameter for dev_command_preview")
		}
		if workingDir == "" && a.terminalContext != nil {
			if tCtx, ok := a.terminalContext.Get(); ok && tCtx.WorkingDir != "" {
				workingDir = tCtx.WorkingDir
			}
		}
		if workingDir == "" && a.editorContext != nil {
			if eCtx, ok := a.editorContext.Get(); ok && eCtx.WorkspaceRoot != "" {
				workingDir = eCtx.WorkspaceRoot
			}
		}
		prev, err := a.devPreviewEngine.Preview(ctx, command, workingDir)
		if err != nil {
			return nil, fmt.Errorf("preview command: %w", err)
		}
		return prev, nil

	case "dev_command_execute":
		if a.devPreviewEngine == nil {
			return nil, errors.New("dev preview engine is not configured")
		}
		var command, workingDir, previewID string
		var confirm bool
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if c, ok := params["command"].(string); ok {
				command = c
			}
			if wd, ok := params["working_dir"].(string); ok {
				workingDir = wd
			}
			if pid, ok := params["preview_id"].(string); ok {
				previewID = pid
			}
			if conf, ok := params["confirm"].(bool); ok {
				confirm = conf
			}
		}
		if previewID != "" {
			res, err := a.devPreviewEngine.ExecutePreview(ctx, previewID, confirm)
			if err != nil {
				return res, fmt.Errorf("execute preview: %w", err)
			}
			return res, nil
		}
		if strings.TrimSpace(command) == "" {
			return nil, errors.New("missing 'command' or 'preview_id' parameter for dev_command_execute")
		}
		if workingDir == "" && a.terminalContext != nil {
			if tCtx, ok := a.terminalContext.Get(); ok && tCtx.WorkingDir != "" {
				workingDir = tCtx.WorkingDir
			}
		}
		if workingDir == "" && a.editorContext != nil {
			if eCtx, ok := a.editorContext.Get(); ok && eCtx.WorkspaceRoot != "" {
				workingDir = eCtx.WorkspaceRoot
			}
		}
		res, err := a.devPreviewEngine.Execute(ctx, command, workingDir, confirm)
		if err != nil {
			return res, fmt.Errorf("execute command: %w", err)
		}
		return res, nil

	case "git_commit_generate":
		if a.gitAssistant == nil {
			return nil, errors.New("git assistant is not configured")
		}
		var workingDir, hint string
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if wd, ok := params["working_dir"].(string); ok {
				workingDir = wd
			}
			if h, ok := params["spoken_hint"].(string); ok {
				hint = h
			}
		}
		if workingDir == "" && a.terminalContext != nil {
			if tCtx, ok := a.terminalContext.Get(); ok && tCtx.WorkingDir != "" {
				workingDir = tCtx.WorkingDir
			}
		}
		if workingDir == "" && a.editorContext != nil {
			if eCtx, ok := a.editorContext.Get(); ok && eCtx.WorkspaceRoot != "" {
				workingDir = eCtx.WorkspaceRoot
			}
		}
		commitRes, err := a.gitAssistant.GenerateCommit(ctx, developer.CommitGenRequest{
			WorkingDir: workingDir,
			SpokenHint: hint,
		})
		if err != nil {
			return nil, fmt.Errorf("generate git commit: %w", err)
		}
		return commitRes, nil

	case "git_pr_generate":
		if a.gitAssistant == nil {
			return nil, errors.New("git assistant is not configured")
		}
		var workingDir, baseBranch, notes string
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if wd, ok := params["working_dir"].(string); ok {
				workingDir = wd
			}
			if b, ok := params["base_branch"].(string); ok {
				baseBranch = b
			}
			if n, ok := params["spoken_notes"].(string); ok {
				notes = n
			}
		}
		if workingDir == "" && a.terminalContext != nil {
			if tCtx, ok := a.terminalContext.Get(); ok && tCtx.WorkingDir != "" {
				workingDir = tCtx.WorkingDir
			}
		}
		if workingDir == "" && a.editorContext != nil {
			if eCtx, ok := a.editorContext.Get(); ok && eCtx.WorkspaceRoot != "" {
				workingDir = eCtx.WorkspaceRoot
			}
		}
		prRes, err := a.gitAssistant.GeneratePR(ctx, developer.PRGenRequest{
			WorkingDir:  workingDir,
			BaseBranch:  baseBranch,
			SpokenNotes: notes,
		})
		if err != nil {
			return nil, fmt.Errorf("generate git PR: %w", err)
		}
		return prRes, nil

	case "dev_code_format":
		var text, style, lang string
		var isDoc, acr bool
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if t, ok := params["text"].(string); ok {
				text = t
			}
			if s, ok := params["style"].(string); ok {
				style = s
			}
			if l, ok := params["language"].(string); ok {
				lang = l
			}
			if d, ok := params["is_docstring"].(bool); ok {
				isDoc = d
			}
			if a, ok := params["preserve_acronyms"].(bool); ok {
				acr = a
			}
		}
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("missing 'text' parameter for dev_code_format")
		}
		if lang == "" && a.editorContext != nil {
			if eCtx, ok := a.editorContext.Get(); ok && eCtx.LanguageID != "" {
				lang = eCtx.LanguageID
			}
		}
		formatRes := developer.FormatCode(developer.CodeFormatRequest{
			Text:             text,
			Style:            developer.CasingStyle(style),
			Language:         lang,
			IsDocstring:      isDoc,
			PreserveAcronyms: acr,
		})
		return formatRes, nil

	case "selection_get":
		var res selection.Result
		var err error
		if a.capturer != nil {
			res, err = a.capturer.Capture(ctx)
		}
		if (err != nil || strings.TrimSpace(res.Text) == "") && a.editorContext != nil {
			if eCtx, ok := a.editorContext.Get(); ok && strings.TrimSpace(eCtx.SelectionText) != "" {
				res = selection.Result{
					Text:        eCtx.SelectionText,
					Method:      "editor_context",
					Application: eCtx.EditorName,
				}
				err = nil
			}
		}
		if err != nil {
			return nil, err
		}
		if a.appDetector != nil && res.Application == "" {
			if info, aErr := a.appDetector.Detect(ctx); aErr == nil {
				res.Application = info.Name
			}
		}
		return res, nil

	case "edit_parse":
		if a.editParser == nil {
			return nil, errors.New("edit parser is not configured")
		}
		var text string
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if t, ok := params["text"].(string); ok {
				text = t
			}
		}
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("missing 'text' parameter for edit_parse")
		}
		instr := a.editParser.Parse(ctx, text)
		return instr, nil

	case "edit_execute":
		if a.editor == nil {
			return nil, errors.New("editor is not configured")
		}
		var instruction, text string
		var dryRun bool
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if ins, ok := params["instruction"].(string); ok {
				instruction = ins
			}
			if t, ok := params["text"].(string); ok {
				text = t
			}
			if dr, ok := params["dry_run"].(bool); ok {
				dryRun = dr
			}
		}
		if strings.TrimSpace(instruction) == "" {
			return nil, errors.New("missing 'instruction' parameter for edit_execute")
		}

		if strings.TrimSpace(text) == "" {
			if a.capturer != nil {
				if sel, err := a.capturer.Capture(ctx); err == nil && strings.TrimSpace(sel.Text) != "" {
					text = sel.Text
				}
			}
			if strings.TrimSpace(text) == "" && a.editorContext != nil {
				if eCtx, ok := a.editorContext.Get(); ok && strings.TrimSpace(eCtx.SelectionText) != "" {
					text = eCtx.SelectionText
				}
			}
		}

		if strings.TrimSpace(text) == "" {
			return nil, errors.New("no text provided and no active selection found")
		}

		var instr edit.Instruction
		if a.editParser != nil {
			instr = a.editParser.Parse(ctx, instruction)
		} else {
			instr = edit.Instruction{
				Raw:        instruction,
				Intent:     edit.IntentCustom,
				Prompt:     instruction,
				IsEdit:     true,
				Confidence: 1.0,
			}
		}

		res, err := a.editor.Execute(ctx, text, instr)
		if err != nil {
			return nil, fmt.Errorf("execute edit transformation: %w", err)
		}

		pasted := false
		if !dryRun && a.paster != nil {
			if _, pErr := a.paster.Paste(res.EditedText); pErr == nil {
				pasted = true
				a.mu.Lock()
				a.recordUndoLocked(&UndoRecord{
					SessionID:      fmt.Sprintf("edit-%d", time.Now().UnixNano()),
					RawText:        text,
					UnpolishedText: text,
					PolishedText:   res.EditedText,
					IsPolished:     true,
					PastedAt:       time.Now(),
					Undone:         false,
				})
				a.mu.Unlock()
			}
		}

		return map[string]any{
			"original_text": res.OriginalText,
			"edited_text":   res.EditedText,
			"instruction":   res.Instruction,
			"dry_run":       dryRun,
			"pasted":        pasted,
			"model":         res.Model,
			"latency_ms":    res.Latency.Milliseconds(),
		}, nil

	case "edit_preview":
		if a.editor == nil {
			return nil, errors.New("editor is not configured")
		}
		var instruction, text string
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if ins, ok := params["instruction"].(string); ok {
				instruction = ins
			}
			if t, ok := params["text"].(string); ok {
				text = t
			}
		}
		if strings.TrimSpace(instruction) == "" {
			return nil, errors.New("missing 'instruction' parameter for edit_preview")
		}
		if strings.TrimSpace(text) == "" {
			if a.capturer != nil {
				if sel, err := a.capturer.Capture(ctx); err == nil && strings.TrimSpace(sel.Text) != "" {
					text = sel.Text
				}
			}
			if strings.TrimSpace(text) == "" && a.editorContext != nil {
				if eCtx, ok := a.editorContext.Get(); ok && strings.TrimSpace(eCtx.SelectionText) != "" {
					text = eCtx.SelectionText
				}
			}
		}
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("no text provided and no active selection found")
		}

		var instr edit.Instruction
		if a.editParser != nil {
			instr = a.editParser.Parse(ctx, instruction)
		} else {
			instr = edit.Instruction{
				Raw:        instruction,
				Intent:     edit.IntentCustom,
				Prompt:     instruction,
				IsEdit:     true,
				Confidence: 1.0,
			}
		}

		res, err := a.editor.Execute(ctx, text, instr)
		if err != nil {
			return nil, fmt.Errorf("execute edit transformation: %w", err)
		}

		sess := a.previewMgr.Create(text, res.EditedText, instr)
		return sess, nil

	case "edit_apply":
		var id string
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if s, ok := params["id"].(string); ok {
				id = s
			}
		}
		if id == "" {
			return nil, errors.New("missing 'id' parameter for edit_apply")
		}

		sess, err := a.previewMgr.Apply(id)
		if err != nil {
			return nil, err
		}

		if a.paster != nil {
			if _, pErr := a.paster.Paste(sess.CandidateText); pErr != nil {
				return nil, fmt.Errorf("paste applied text: %w", pErr)
			}
			a.mu.Lock()
			a.recordUndoLocked(&UndoRecord{
				SessionID:      fmt.Sprintf("edit-%d", time.Now().UnixNano()),
				RawText:        sess.OriginalText,
				UnpolishedText: sess.OriginalText,
				PolishedText:   sess.CandidateText,
				IsPolished:     true,
				PastedAt:       time.Now(),
				Undone:         false,
			})
			a.mu.Unlock()
		}
		return sess, nil

	case "edit_reject":
		var id string
		if params, ok := req.Params.(map[string]any); ok && params != nil {
			if s, ok := params["id"].(string); ok {
				id = s
			}
		}
		if id == "" {
			return nil, errors.New("missing 'id' parameter for edit_reject")
		}
		sess, err := a.previewMgr.Reject(id)
		if err != nil {
			return nil, err
		}
		return sess, nil

	case "edit_preview_list":
		return a.previewMgr.ListPending(), nil
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

	case "polish_route_list":
		if a.router == nil {
			return map[string]string{}, nil
		}
		return a.router.ListRoutes(), nil

	case "polish_route_set":
		params, _ := req.Params.(map[string]any)
		app, _ := params["app"].(string)
		profile, _ := params["profile"].(string)
		if strings.TrimSpace(app) == "" || strings.TrimSpace(profile) == "" {
			return nil, errors.New("missing app or profile parameter")
		}
		if a.router == nil {
			a.router = polish.NewRouter(nil)
		}
		a.router.SetRoute(app, profile)
		return map[string]any{"ok": true, "app": app, "profile": profile}, nil

	case "polish_route_delete":
		params, _ := req.Params.(map[string]any)
		app, _ := params["app"].(string)
		if strings.TrimSpace(app) == "" {
			return nil, errors.New("missing app parameter")
		}
		if a.router != nil {
			a.router.DeleteRoute(app)
		}
		return map[string]any{"ok": true, "app": app}, nil

	case "polish_route_test":
		params, _ := req.Params.(map[string]any)
		app, _ := params["app"].(string)
		if strings.TrimSpace(app) == "" {
			return nil, errors.New("missing app parameter")
		}
		cat := appcontext.Categorize(app, "", "")
		routed := "general"
		if a.router != nil {
			routed = a.router.Route(app, cat)
		}
		return map[string]any{
			"app":      app,
			"category": string(cat),
			"profile":  routed,
		}, nil

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
