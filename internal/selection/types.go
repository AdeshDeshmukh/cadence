package selection

import (
	"context"
	"errors"
)

var (
	// ErrNoSelection is returned when no text is currently highlighted or selected.
	ErrNoSelection = errors.New("no text selected in the active application")
)

// Result represents the captured text selection and metadata.
type Result struct {
	Text        string `json:"text"`
	Method      string `json:"method"`
	Application string `json:"application,omitempty"`
}

// Capturer defines the contract for retrieving active text selection.
type Capturer interface {
	Capture(ctx context.Context) (Result, error)
}
