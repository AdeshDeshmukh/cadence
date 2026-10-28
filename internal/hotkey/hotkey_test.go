package hotkey

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestParseShortcut_Valid(t *testing.T) {
	cases := []struct {
		input       string
		expectedCmd bool
		expectedAlt bool
		expectedKey string
	}{
		{"Cmd+Shift+Space", true, false, "SPACE"},
		{"Command+Option+D", true, true, "D"},
		{"Ctrl+Alt+Delete", false, true, "DELETE"},
		{"Option", false, true, ""},
		{"Fn", false, false, ""},
	}

	for _, tc := range cases {
		sc, err := ParseShortcut(tc.input)
		if err != nil {
			t.Fatalf("unexpected error parsing %q: %v", tc.input, err)
		}
		if sc.Modifiers.Cmd != tc.expectedCmd {
			t.Fatalf("expected Cmd=%v for %q, got %v", tc.expectedCmd, tc.input, sc.Modifiers.Cmd)
		}
		if sc.Modifiers.Alt != tc.expectedAlt {
			t.Fatalf("expected Alt=%v for %q, got %v", tc.expectedAlt, tc.input, sc.Modifiers.Alt)
		}
		if sc.Key != tc.expectedKey {
			t.Fatalf("expected Key=%q for %q, got %q", tc.expectedKey, tc.input, sc.Key)
		}
	}
}

func TestParseShortcut_Invalid(t *testing.T) {
	badInputs := []string{
		"",
		"   ",
		"A+B", // Multiple primary keys without modifiers
	}

	for _, input := range badInputs {
		_, err := ParseShortcut(input)
		if err == nil {
			t.Fatalf("expected error parsing invalid shortcut %q", input)
		}
	}
}

func TestShortcut_String(t *testing.T) {
	sc, err := ParseShortcut("Cmd+Shift+Space")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	str := sc.String()
	if str != "Cmd+Shift+Space" {
		t.Fatalf("expected canonical string 'Cmd+Shift+Space', got %q", str)
	}
}

func TestSimulatedListener_Lifecycle(t *testing.T) {
	l := NewSimulatedListener("Cmd+Shift+Space", ModePushToTalk)
	if l.IsListening() {
		t.Fatalf("expected not listening initially")
	}

	// Double stop returns ErrNotListening
	if err := l.Stop(); !errors.Is(err, ErrNotListening) {
		t.Fatalf("expected ErrNotListening on early stop, got %v", err)
	}

	// Start
	ch, err := l.Start()
	if err != nil {
		t.Fatalf("unexpected error starting: %v", err)
	}
	if !l.IsListening() {
		t.Fatalf("expected listening after Start()")
	}

	// Double start returns ErrAlreadyListening
	if _, err := l.Start(); !errors.Is(err, ErrAlreadyListening) {
		t.Fatalf("expected ErrAlreadyListening, got %v", err)
	}

	// Emit key down and key up
	go func() {
		l.Emit(EventKeyDown)
		time.Sleep(10 * time.Millisecond)
		l.Emit(EventKeyUp)
	}()

	ev1 := <-ch
	if ev1.Type != EventKeyDown || ev1.Shortcut != "Cmd+Shift+Space" {
		t.Fatalf("unexpected event 1: %+v", ev1)
	}

	ev2 := <-ch
	if ev2.Type != EventKeyUp || ev2.Shortcut != "Cmd+Shift+Space" {
		t.Fatalf("unexpected event 2: %+v", ev2)
	}

	// Stop
	if err := l.Stop(); err != nil {
		t.Fatalf("unexpected error stopping: %v", err)
	}

	if l.IsListening() {
		t.Fatalf("expected not listening after Stop()")
	}
}

func TestSimulatedListener_ModeAndHold(t *testing.T) {
	l := NewSimulatedListener("", "")
	if l.Mode() != ModePushToTalk {
		t.Fatalf("expected default push to talk, got %s", l.Mode())
	}
	if l.Shortcut() != "Cmd+Shift+Space" {
		t.Fatalf("expected default shortcut, got %s", l.Shortcut())
	}

	l.SetMode(ModeToggle)
	if l.Mode() != ModeToggle {
		t.Fatalf("expected mode toggle, got %s", l.Mode())
	}

	ch, _ := l.Start()
	defer l.Stop()

	go l.SimulateHold(20 * time.Millisecond)

	evDown := <-ch
	if evDown.Type != EventKeyDown {
		t.Fatalf("expected key down from hold")
	}

	evUp := <-ch
	if evUp.Type != EventKeyUp {
		t.Fatalf("expected key up from hold")
	}
}

func TestDarwinListener_Basic(t *testing.T) {
	l := NewDarwinListener("Cmd+D", ModePushToTalk)
	ch, err := l.Start()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Stop()

	if !l.IsListening() {
		t.Fatalf("expected listening")
	}

	go l.Emit(EventKeyDown)

	ev := <-ch
	if ev.Type != EventKeyDown || ev.Shortcut != "Cmd+D" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestListener_ConcurrentRace(t *testing.T) {
	l := NewSimulatedListener("Cmd+Shift+Space", ModePushToTalk)
	ch, err := l.Start()
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer l.Stop()

	var wg sync.WaitGroup
	numGoroutines := 8
	iterations := 100

	// Concurrent emitters
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				l.Emit(EventKeyDown)
				_ = l.IsListening()
				_ = l.Mode()
				_ = l.Shortcut()
				if j%20 == 0 {
					l.SetMode(ModeToggle)
				} else if j%20 == 10 {
					l.SetMode(ModePushToTalk)
				}
			}
		}(i)
	}

	// Concurrent reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		received := 0
		for range ch {
			received++
			if received >= 200 {
				return
			}
		}
	}()

	wg.Wait()
}
