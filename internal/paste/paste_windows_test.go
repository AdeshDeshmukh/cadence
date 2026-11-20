//go:build windows

package paste

import (
	"testing"
)

func TestEscapeSendKeys(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello world"},
		{"a+b=c", "a{+}b=c"},
		{"git commit -m 'feat^5'", "git commit -m 'feat{^}5'"},
		{"100%", "100{%}"},
		{"~home", "{~}home"},
		{"func()", "func{(}{)}"},
		{"array[0]", "array{[}0{]}"},
		{"{key: val}", "{{}key: val{}}"},
	}

	for _, tc := range cases {
		out := escapeSendKeys(tc.input)
		if out != tc.expected {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, out)
		}
	}
}

func TestWindowsTiers_Names(t *testing.T) {
	t1 := NewWin32TypeTier()
	if t1.Name() != "win32_keystroke_type" {
		t.Errorf("unexpected name: %s", t1.Name())
	}

	t2 := NewWin32ClipboardTier()
	if t2.Name() != "win32_clipboard_keystroke" {
		t.Errorf("unexpected name: %s", t2.Name())
	}

	t3 := NewFallbackMemoryTier()
	if t3.Name() != "fallback_memory" {
		t.Errorf("unexpected name: %s", t3.Name())
	}
}

func TestWindowsClipboard_ReadWrite(t *testing.T) {
	testMsg := "cadence_windows_clipboard_test_789"
	if err := SetSystemClipboard(testMsg); err != nil {
		t.Fatalf("SetSystemClipboard failed: %v", err)
	}

	readBack, err := GetSystemClipboard()
	if err != nil {
		t.Fatalf("GetSystemClipboard failed: %v", err)
	}

	if readBack != testMsg {
		t.Fatalf("expected clipboard text %q, got %q", testMsg, readBack)
	}
}

func TestWindowsCascade_DefaultOrder(t *testing.T) {
	c := NewDefaultCascade()
	tiers := c.Tiers()

	expected := []string{
		"win32_keystroke_type",
		"win32_clipboard_keystroke",
		"fallback_memory",
	}

	if len(tiers) != len(expected) {
		t.Fatalf("expected %d tiers, got %d: %v", len(expected), len(tiers), tiers)
	}

	for i, name := range expected {
		if tiers[i] != name {
			t.Errorf("tier %d: expected %s, got %s", i, name, tiers[i])
		}
	}
}

func TestWindowsCascade_Execution(t *testing.T) {
	c := NewDefaultCascade()
	res, err := c.Paste("hello from windows test")
	if err != nil {
		t.Fatalf("cascade paste failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected res.Success to be true")
	}

	if res.Method == "" {
		t.Errorf("expected non-empty res.Method")
	}
}

func TestWindowsNativeReverter(t *testing.T) {
	reverter := NewNativeReverter()
	_ = reverter.Revert()
}
