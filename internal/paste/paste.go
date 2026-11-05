package paste

import "errors"

type Result struct {
	Method  string
	Success bool
}

type Paster interface {
	Paste(string) (Result, error)
}

// Reverter provides an abstraction to undo the last text paste in the active application.
type Reverter interface {
	Revert() error
}

type Unsupported struct{}

func (Unsupported) Paste(string) (Result, error) {
	return Result{}, errors.New(
		"native paste backend is not installed",
	)
}
