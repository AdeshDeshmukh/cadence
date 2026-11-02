package paste

import (
	"sync"
	"time"
)

// PreservingPaster wraps any Paster to save and restore user clipboard content.
type PreservingPaster struct {
	inner             Paster
	preserveClipboard bool
	restoreDelay      time.Duration
	getClipboard      func() (string, error)
	setClipboard      func(string) error
	mu                sync.Mutex
	activeRestore     sync.WaitGroup
}

// NewPreservingPaster creates a new PreservingPaster wrapper.
func NewPreservingPaster(inner Paster, preserve bool, delay time.Duration) *PreservingPaster {
	if delay <= 0 {
		delay = 80 * time.Millisecond
	}
	return &PreservingPaster{
		inner:             inner,
		preserveClipboard: preserve,
		restoreDelay:      delay,
		getClipboard:      GetSystemClipboard,
		setClipboard:      SetSystemClipboard,
	}
}

// SetClipboardAccessors overrides clipboard getters and setters for testing.
func (p *PreservingPaster) SetClipboardAccessors(getFn func() (string, error), setFn func(string) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.getClipboard = getFn
	p.setClipboard = setFn
}

// Paste executes the paste operation, preserving and restoring previous clipboard if enabled.
func (p *PreservingPaster) Paste(text string) (Result, error) {
	p.mu.Lock()
	preserve := p.preserveClipboard
	delay := p.restoreDelay
	getFn := p.getClipboard
	setFn := p.setClipboard
	p.mu.Unlock()

	var originalClipboard string
	var hasOriginal bool

	if preserve && getFn != nil {
		if content, err := getFn(); err == nil {
			originalClipboard = content
			hasOriginal = true
		}
	}

	result, err := p.inner.Paste(text)
	if err != nil {
		return result, err
	}

	if preserve && hasOriginal && setFn != nil {
		p.activeRestore.Add(1)
		go func(restoreText string) {
			defer p.activeRestore.Done()
			time.Sleep(delay)
			_ = setFn(restoreText)
		}(originalClipboard)
	}

	return result, nil
}

// WaitForRestore blocks until all background clipboard restorations complete.
func (p *PreservingPaster) WaitForRestore() {
	p.activeRestore.Wait()
}
