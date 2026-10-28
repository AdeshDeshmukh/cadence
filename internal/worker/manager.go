package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	ErrWorkerStopped = errors.New("worker is stopped")
	ErrWorkerCrashed = errors.New("worker process crashed unexpectedly")
	ErrCallTimeout   = errors.New("worker call timed out")
)

type Response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	raw   map[string]any
}

func NewResponse(id string, ok bool, raw map[string]any) *Response {
	if raw == nil {
		raw = make(map[string]any)
	}
	return &Response{
		ID:  id,
		OK:  ok,
		raw: raw,
	}
}

func (r *Response) Get(key string) (any, bool) {
	val, ok := r.raw[key]
	return val, ok
}

func (r *Response) GetString(key string) string {
	if val, ok := r.raw[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func (r *Response) GetFloat64(key string) float64 {
	if val, ok := r.raw[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0
}

func (r *Response) UnmarshalResult(dest any) error {
	data, err := json.Marshal(r.raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

type Caller interface {
	Call(ctx context.Context, method string, params map[string]any) (*Response, error)
	Ping(ctx context.Context) error
	Stop() error
}

type Config struct {
	Command             string
	Args                []string
	Env                 []string
	WorkDir             string
	PingTimeout         time.Duration
	InitialRestartDelay time.Duration
	MaxRestartDelay     time.Duration
	MaxRestarts         int
	AutoRestart         bool
}

func DefaultConfig(command string, args ...string) Config {
	return Config{
		Command:             command,
		Args:                args,
		PingTimeout:         2 * time.Second,
		InitialRestartDelay: 200 * time.Millisecond,
		MaxRestartDelay:     10 * time.Second,
		MaxRestarts:         5,
		AutoRestart:         true,
	}
}

type Manager struct {
	cfg Config
	seq uint64

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writeMu sync.Mutex

	pending   map[string]chan *Response
	pendingMu sync.Mutex

	stopCh    chan struct{}
	stopped   bool
	restartCh chan struct{}
}

func NewManager(cfg Config) *Manager {
	if cfg.PingTimeout <= 0 {
		cfg.PingTimeout = 2 * time.Second
	}
	if cfg.InitialRestartDelay <= 0 {
		cfg.InitialRestartDelay = 200 * time.Millisecond
	}
	if cfg.MaxRestartDelay <= 0 {
		cfg.MaxRestartDelay = 10 * time.Second
	}
	if cfg.MaxRestarts <= 0 {
		cfg.MaxRestarts = 5
	}

	return &Manager{
		cfg:       cfg,
		pending:   make(map[string]chan *Response),
		stopCh:    make(chan struct{}),
		restartCh: make(chan struct{}, 1),
	}
}

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return ErrWorkerStopped
	}

	if err := m.spawnLocked(); err != nil {
		return fmt.Errorf("spawn worker: %w", err)
	}

	if m.cfg.AutoRestart {
		go m.supervise()
	}

	if m.cfg.PingTimeout > 0 {
		pingCtx, cancel := context.WithTimeout(ctx, m.cfg.PingTimeout)
		defer cancel()
		if err := m.pingLocked(pingCtx); err != nil {
			_ = m.terminateLocked()
			return fmt.Errorf("worker readiness ping failed: %w", err)
		}
	}

	return nil
}

func (m *Manager) spawnLocked() error {
	cmd := exec.Command(m.cfg.Command, m.cfg.Args...)
	if m.cfg.WorkDir != "" {
		cmd.Dir = m.cfg.WorkDir
	}

	cmd.Env = append(os.Environ(), m.cfg.Env...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return err
	}

	m.cmd = cmd
	m.stdin = stdin

	go m.readLoop(stdout, cmd)

	return nil
}

func (m *Manager) readLoop(stdout io.ReadCloser, cmd *exec.Cmd) {
	defer stdout.Close()

	scanner := bufio.NewScanner(stdout)
	const maxCapacity = 10 * 1024 * 1024
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		id, _ := raw["id"].(string)
		ok, _ := raw["ok"].(bool)
		errMsg, _ := raw["error"].(string)

		resp := &Response{
			ID:    id,
			OK:    ok,
			Error: errMsg,
			raw:   raw,
		}

		m.pendingMu.Lock()
		ch, exists := m.pending[id]
		if exists {
			delete(m.pending, id)
		}
		m.pendingMu.Unlock()

		if exists {
			select {
			case ch <- resp:
			default:
			}
		}
	}

	_ = cmd.Wait()

	m.pendingMu.Lock()
	for id, ch := range m.pending {
		delete(m.pending, id)
		select {
		case ch <- &Response{ID: id, OK: false, Error: ErrWorkerCrashed.Error()}:
		default:
		}
	}
	m.pendingMu.Unlock()

	m.mu.Lock()
	stopped := m.stopped
	m.mu.Unlock()

	if !stopped && m.cfg.AutoRestart {
		select {
		case m.restartCh <- struct{}{}:
		default:
		}
	}
}

func (m *Manager) supervise() {
	delay := m.cfg.InitialRestartDelay
	restarts := 0
	lastSpawn := time.Now()

	for {
		select {
		case <-m.stopCh:
			return
		case <-m.restartCh:
			m.mu.Lock()
			if m.stopped {
				m.mu.Unlock()
				return
			}

			if time.Since(lastSpawn) > 10*time.Second {
				restarts = 0
				delay = m.cfg.InitialRestartDelay
			}

			if restarts >= m.cfg.MaxRestarts {
				m.mu.Unlock()
				return
			}
			restarts++
			m.mu.Unlock()

			select {
			case <-m.stopCh:
				return
			case <-time.After(delay):
			}

			delay *= 2
			if delay > m.cfg.MaxRestartDelay {
				delay = m.cfg.MaxRestartDelay
			}

			m.mu.Lock()
			if !m.stopped {
				if err := m.spawnLocked(); err != nil {
					if restarts < m.cfg.MaxRestarts {
						select {
						case m.restartCh <- struct{}{}:
						default:
						}
					}
				} else {
					lastSpawn = time.Now()
				}
			}
			m.mu.Unlock()
		}
	}
}

func (m *Manager) Ping(ctx context.Context) error {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return ErrWorkerStopped
	}
	if m.cmd == nil || m.stdin == nil {
		m.mu.Unlock()
		return ErrWorkerCrashed
	}
	stdin := m.stdin
	m.mu.Unlock()

	resp, err := m.executeCall(ctx, stdin, "ping", nil)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("ping rejected: %s", resp.Error)
	}
	return nil
}

func (m *Manager) pingLocked(ctx context.Context) error {
	if m.stopped {
		return ErrWorkerStopped
	}
	if m.cmd == nil || m.stdin == nil {
		return ErrWorkerCrashed
	}
	resp, err := m.executeCall(ctx, m.stdin, "ping", nil)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("ping rejected: %s", resp.Error)
	}
	return nil
}

func (m *Manager) Call(ctx context.Context, method string, params map[string]any) (*Response, error) {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return nil, ErrWorkerStopped
	}
	if m.cmd == nil || m.stdin == nil {
		m.mu.Unlock()
		return nil, ErrWorkerCrashed
	}
	stdin := m.stdin
	m.mu.Unlock()

	return m.executeCall(ctx, stdin, method, params)
}

func (m *Manager) executeCall(ctx context.Context, stdin io.Writer, method string, params map[string]any) (*Response, error) {
	reqID := fmt.Sprintf("req-%d", atomic.AddUint64(&m.seq, 1))

	reqData := make(map[string]any)
	for k, v := range params {
		reqData[k] = v
	}
	reqData["id"] = reqID
	reqData["method"] = method

	line, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	line = append(line, '\n')

	respCh := make(chan *Response, 1)

	m.pendingMu.Lock()
	m.pending[reqID] = respCh
	m.pendingMu.Unlock()

	m.writeMu.Lock()
	_, writeErr := stdin.Write(line)
	m.writeMu.Unlock()

	if writeErr != nil {
		m.pendingMu.Lock()
		delete(m.pending, reqID)
		m.pendingMu.Unlock()
		return nil, fmt.Errorf("write request: %w", writeErr)
	}

	select {
	case <-ctx.Done():
		m.pendingMu.Lock()
		delete(m.pending, reqID)
		m.pendingMu.Unlock()
		return nil, ctx.Err()
	case resp := <-respCh:
		if !resp.OK && resp.Error != "" {
			return resp, fmt.Errorf("worker error: %s", resp.Error)
		}
		return resp, nil
	}
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.terminateLocked()
}

func (m *Manager) terminateLocked() error {
	if m.stopped {
		return nil
	}
	m.stopped = true
	close(m.stopCh)

	if m.stdin != nil {
		_ = m.stdin.Close()
	}

	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Signal(syscall.SIGTERM)

		done := make(chan struct{})
		go func(proc *os.Process) {
			_, _ = proc.Wait()
			close(done)
		}(m.cmd.Process)

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			_ = m.cmd.Process.Kill()
		}
	}

	return nil
}
