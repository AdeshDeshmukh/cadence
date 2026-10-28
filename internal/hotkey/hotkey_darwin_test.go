//go:build darwin

package hotkey

import (
	"testing"
)

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
