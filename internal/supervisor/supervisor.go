package supervisor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"time"
)

var (
	// ErrCrashLoopDetected is returned when process crashes repeatedly within the sliding window.
	ErrCrashLoopDetected = errors.New("supervisor: rapid crash loop detected, maximum restart threshold reached")

	// ErrCleanShutdown indicates the process stopped cleanly upon request.
	ErrCleanShutdown = errors.New("supervisor: process stopped cleanly")
)

// Config configures supervisor auto-restart and backoff behavior.
type Config struct {
	MaxRestarts  int           // Maximum restarts allowed in Window before giving up (default 5)
	Window       time.Duration // Time window for counting restarts (default 60s)
	InitialDelay time.Duration // Initial sleep before first restart (default 100ms)
	MaxDelay     time.Duration // Maximum capped sleep between restarts (default 10s)
	Multiplier   float64       // Exponential factor (default 2.0)
	CrashDir     string        // Path to persist crash dump files
}

// DefaultConfig returns recommended production supervisor settings.
func DefaultConfig() Config {
	return Config{
		MaxRestarts:  5,
		Window:       60 * time.Second,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		CrashDir:     DefaultCrashDir(),
	}
}

// Supervisor manages long-running processes with fault detection and auto-restart.
type Supervisor struct {
	cfg      Config
	mu       sync.Mutex
	restarts []time.Time
}

// New creates a new Supervisor instance.
func New(cfg Config) *Supervisor {
	if cfg.MaxRestarts <= 0 {
		cfg.MaxRestarts = 5
	}
	if cfg.Window <= 0 {
		cfg.Window = 60 * time.Second
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 10 * time.Second
	}
	if cfg.Multiplier <= 1.0 {
		cfg.Multiplier = 2.0
	}
	if cfg.CrashDir == "" {
		cfg.CrashDir = DefaultCrashDir()
	}

	return &Supervisor{
		cfg: cfg,
	}
}

// Run executes runFn under supervision, automatically recovering from panics and restarting on abnormal exit.
func (s *Supervisor) Run(ctx context.Context, runFn func(context.Context) error) error {
	attempt := 0

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var runErr error
		var panicked bool
		var panicVal any
		var stackTrace string

		func() {
			defer func() {
				if r := recover(); r != nil {
					panicked = true
					panicVal = r
					buf := make([]byte, 16*1024)
					n := runtime.Stack(buf, false)
					stackTrace = string(buf[:n])
				}
			}()

			runErr = runFn(ctx)
		}()

		// Normal graceful shutdown
		if !panicked && (runErr == nil || errors.Is(runErr, context.Canceled)) {
			return nil
		}

		// Process crashed or exited with an unexpected error
		now := time.Now().UTC()
		s.mu.Lock()
		s.restarts = append(s.restarts, now)
		// Prune restarts older than sliding window
		cutoff := now.Add(-s.cfg.Window)
		validIdx := 0
		for _, t := range s.restarts {
			if t.After(cutoff) {
				s.restarts[validIdx] = t
				validIdx++
			}
		}
		s.restarts = s.restarts[:validIdx]
		recentCount := len(s.restarts)
		s.mu.Unlock()

		attempt++

		// Capture diagnostic crash report
		reason := "error"
		errStr := ""
		if panicked {
			reason = "panic"
			errStr = fmt.Sprintf("panic: %v", panicVal)
			log.Printf("[SUPERVISOR CRASH] Process panicked: %v\nStack trace:\n%s", panicVal, stackTrace)
		} else if runErr != nil {
			errStr = runErr.Error()
			log.Printf("[SUPERVISOR CRASH] Process exited unexpectedly with error: %v", runErr)
		}

		report := CaptureCrash(reason, errStr, stackTrace, attempt)
		if path, dumpErr := WriteCrashDump(s.cfg.CrashDir, &report); dumpErr == nil {
			log.Printf("[SUPERVISOR] Crash dump saved to %s", path)
		}

		// Check crash loop breaker threshold
		if recentCount >= s.cfg.MaxRestarts {
			log.Printf("[SUPERVISOR FATAL] %d crashes detected in %v window (max %d). Halting restart loop.",
				recentCount, s.cfg.Window, s.cfg.MaxRestarts)
			return fmt.Errorf("%w: %d crashes in %v", ErrCrashLoopDetected, recentCount, s.cfg.Window)
		}

		// Calculate exponential backoff sleep with jitter
		delay := s.calculateDelay(attempt)
		log.Printf("[SUPERVISOR] Auto-restarting process in %v (attempt #%d)...", delay, attempt)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Proceed to restart
		}
	}
}

func (s *Supervisor) calculateDelay(attempt int) time.Duration {
	delay := float64(s.cfg.InitialDelay)
	for i := 1; i < attempt; i++ {
		delay *= s.cfg.Multiplier
		if delay >= float64(s.cfg.MaxDelay) {
			delay = float64(s.cfg.MaxDelay)
			break
		}
	}

	// Add 10-25% random jitter to avoid thundering herds
	jitter := 0.10 + rand.Float64()*0.15
	delay = delay * (1.0 + jitter)

	if time.Duration(delay) > s.cfg.MaxDelay {
		return s.cfg.MaxDelay
	}
	return time.Duration(delay)
}

// RecentCrashes returns all saved crash dumps.
func (s *Supervisor) RecentCrashes() ([]CrashReport, error) {
	return ReadCrashDumps(s.cfg.CrashDir)
}

// RestartCount returns total restarts recorded in the active sliding window.
func (s *Supervisor) RestartCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.restarts)
}
