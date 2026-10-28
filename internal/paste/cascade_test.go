package paste

import (
	"errors"
	"sync"
	"testing"
)

type mockTier struct {
	mu        sync.Mutex
	name      string
	shouldErr bool
	called    bool
	text      string
}

func (m *mockTier) Name() string {
	return m.name
}

func (m *mockTier) Paste(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called = true
	m.text = text
	if m.shouldErr {
		return errors.New("tier simulation failure")
	}
	return nil
}

func TestCascade_EmptyText(t *testing.T) {
	c := NewCascade(&mockTier{name: "m1"})
	_, err := c.Paste("")
	if !errors.Is(err, ErrEmptyText) {
		t.Fatalf("expected ErrEmptyText on empty paste, got %v", err)
	}
}

func TestCascade_NoTiers(t *testing.T) {
	c := NewCascade()
	_, err := c.Paste("test text")
	if !errors.Is(err, ErrAllTiersFailed) {
		t.Fatalf("expected ErrAllTiersFailed when no tiers registered, got %v", err)
	}
}

func TestCascade_FirstTierSucceeds(t *testing.T) {
	t1 := &mockTier{name: "tier1"}
	t2 := &mockTier{name: "tier2"}
	c := NewCascade(t1, t2)

	res, err := c.Paste("hello cadence")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Method != "tier1" || !res.Success {
		t.Fatalf("expected tier1 success, got %+v", res)
	}
	if !t1.called {
		t.Fatalf("expected tier1 called")
	}
	if t2.called {
		t.Fatalf("expected tier2 NOT called when tier1 succeeded")
	}
}

func TestCascade_FallbackToSecondTier(t *testing.T) {
	t1 := &mockTier{name: "tier1", shouldErr: true}
	t2 := &mockTier{name: "tier2", shouldErr: false}
	t3 := &mockTier{name: "tier3", shouldErr: false}
	c := NewCascade(t1, t2, t3)

	res, err := c.Paste("fallback test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Method != "tier2" || !res.Success {
		t.Fatalf("expected tier2 success, got %+v", res)
	}
	if !t1.called || !t2.called {
		t.Fatalf("expected tier1 and tier2 both called")
	}
	if t3.called {
		t.Fatalf("expected tier3 NOT called once tier2 succeeded")
	}
}

func TestCascade_AllTiersFail(t *testing.T) {
	t1 := &mockTier{name: "t1", shouldErr: true}
	t2 := &mockTier{name: "t2", shouldErr: true}
	c := NewCascade(t1, t2)

	_, err := c.Paste("will fail")
	if !errors.Is(err, ErrAllTiersFailed) {
		t.Fatalf("expected ErrAllTiersFailed when all tiers fail, got %v", err)
	}
}

func TestCascade_AddTierAndNames(t *testing.T) {
	c := NewCascade()
	c.AddTier(&mockTier{name: "alpha"})
	c.AddTier(&mockTier{name: "beta"})

	names := c.Tiers()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("unexpected tier names: %v", names)
	}
}

func TestNewDefaultCascade(t *testing.T) {
	c := NewDefaultCascade()
	names := c.Tiers()
	if len(names) == 0 {
		t.Fatalf("expected at least 1 tier in default cascade")
	}
}

func TestCascade_ConcurrentRace(t *testing.T) {
	c := NewCascade(
		&mockTier{name: "t1", shouldErr: true},
		&mockTier{name: "t2", shouldErr: false},
	)

	var wg sync.WaitGroup
	numGoroutines := 8
	iterations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = c.Paste("concurrent text")
				_ = c.Tiers()
			}
		}(i)
	}

	wg.Wait()
}
