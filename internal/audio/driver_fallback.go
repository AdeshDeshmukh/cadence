//go:build (!darwin && !linux && !windows) || (darwin && !cgo)

package audio

import "time"

// NewDefaultStreamDriver returns a simulated driver on non-Darwin/non-Linux/non-Windows or non-cgo systems.
func NewDefaultStreamDriver() StreamDriver {
	return NewSimulatedDriver(440, 512, 32*time.Millisecond)
}
