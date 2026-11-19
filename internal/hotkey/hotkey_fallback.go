//go:build !darwin && !linux

package hotkey

// NewDefaultListener creates the fallback listener for non-macOS and non-Linux systems.
func NewDefaultListener(shortcut string, mode Mode) Listener {
	return NewSimulatedListener(shortcut, mode)
}
