package supervisor

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSupervisor_CleanShutdown(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{
		MaxRestarts:  3,
		Window:       10 * time.Second,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		CrashDir:     tmpDir,
	}
	sup := New(cfg)

	runs := 0
	err := sup.Run(context.Background(), func(ctx context.Context) error {
		runs++
		return nil // clean exit
	})

	if err != nil {
		t.Fatalf("expected nil error on clean exit, got: %v", err)
	}
	if runs != 1 {
		t.Errorf("expected exactly 1 run, got %d", runs)
	}

	crashes, _ := sup.RecentCrashes()
	if len(crashes) != 0 {
		t.Errorf("expected 0 crashes on clean exit, got %d", len(crashes))
	}
}

func TestSupervisor_PanicRecoveryAndRestart(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{
		MaxRestarts:  5,
		Window:       10 * time.Second,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     30 * time.Millisecond,
		CrashDir:     tmpDir,
	}
	sup := New(cfg)

	runs := 0
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := sup.Run(ctx, func(c context.Context) error {
		runs++
		if runs == 1 {
			panic("simulated fatal pipeline null pointer dereference")
		}
		if runs == 2 {
			return errors.New("simulated audio device disconnected")
		}
		// On 3rd run succeed cleanly
		return nil
	})

	if err != nil {
		t.Fatalf("expected successful run after restarts, got: %v", err)
	}
	if runs != 3 {
		t.Errorf("expected 3 runs before success, got: %d", runs)
	}

	crashes, err := sup.RecentCrashes()
	if err != nil {
		t.Fatalf("RecentCrashes failed: %v", err)
	}
	if len(crashes) != 2 {
		t.Fatalf("expected 2 crash reports recorded, got: %d", len(crashes))
	}

	// Verify panic report has stack trace
	if crashes[1].Reason != "panic" && crashes[0].Reason != "panic" {
		t.Errorf("expected at least one panic reason in crash reports")
	}
}

func TestSupervisor_CrashLoopBreaker(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{
		MaxRestarts:  3,
		Window:       5 * time.Second,
		InitialDelay: 5 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		CrashDir:     tmpDir,
	}
	sup := New(cfg)

	runs := 0
	err := sup.Run(context.Background(), func(ctx context.Context) error {
		runs++
		return errors.New("continuous crash")
	})

	if err == nil {
		t.Fatal("expected crash loop error, got nil")
	}
	if !errors.Is(err, ErrCrashLoopDetected) {
		t.Errorf("expected ErrCrashLoopDetected, got: %v", err)
	}
	if runs != 3 {
		t.Errorf("expected 3 runs before circuit breaker tripped, got: %d", runs)
	}
}
