package paste

import "errors"

type Result struct {
	Method  string
	Success bool
}

type Paster interface {
	Paste(string) (Result, error)
}

type Unsupported struct{}

func (Unsupported) Paste(string) (Result, error) {
	return Result{}, errors.New(
		"native paste backend is not installed",
	)
}
