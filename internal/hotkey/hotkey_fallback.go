//go:build !darwin && !linux && !windows

package hotkey

// NewDefaultListener creates the fallback listener for non-macOS, non-Linux, and non-Windows systems.
func NewDefaultListener(shortcut string, mode Mode) Listener {
	return NewSimulatedListener(shortcut, mode)
}
