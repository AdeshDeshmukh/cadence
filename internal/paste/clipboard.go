package paste

import "sync"

type Clipboard struct {
	mu   sync.RWMutex
	text string
}

func NewClipboard() *Clipboard {
	return &Clipboard{}
}

func (c *Clipboard) Paste(text string) (Result, error) {
	c.mu.Lock()
	c.text = text
	c.mu.Unlock()
	return Result{Method: "memory", Success: true}, nil
}

func (c *Clipboard) Text() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.text
}
