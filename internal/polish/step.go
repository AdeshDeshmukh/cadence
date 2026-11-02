package polish

import (
	"context"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
)

// Step adapts a Polisher into a postprocess.Step.
type Step struct {
	polisher Polisher
	profile  string
	timeout  time.Duration
}

// NewStep creates a post-processing step wrapping the given Polisher.
func NewStep(polisher Polisher, profile string, timeout time.Duration) *Step {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Step{
		polisher: polisher,
		profile:  profile,
		timeout:  timeout,
	}
}

func (s *Step) Name() string {
	return "polish"
}

func (s *Step) Apply(text string, ctx postprocess.Context) string {
	if s.polisher == nil || strings.TrimSpace(text) == "" {
		return text
	}

	profile := s.profile
	if profile == "" {
		profile = ctx.Mode
	}
	if profile == "" {
		profile = "general"
	}

	req := Request{
		Text:        text,
		Profile:     profile,
		Application: ctx.Application,
	}

	callCtx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	type callResult struct {
		resp Response
		err  error
	}

	resultCh := make(chan callResult, 1)
	go func() {
		r, err := s.polisher.Polish(callCtx, req)
		resultCh <- callResult{resp: r, err: err}
	}()

	select {
	case <-callCtx.Done():
		// Timeout exceeded: immediately fall back to original text
		return text
	case res := <-resultCh:
		if res.err != nil || strings.TrimSpace(res.resp.Text) == "" {
			return text
		}
		return res.resp.Text
	}
}
