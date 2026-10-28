//go:build !darwin

package hotkey

// NewDefaultListener creates the fallback listener for non-macOS systems.
func NewDefaultListener(shortcut string, mode Mode) Listener {
	return NewSimulatedListener(shortcut, mode)
}
