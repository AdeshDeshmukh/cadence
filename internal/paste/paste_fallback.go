//go:build !darwin

package paste

// FallbackMemoryTier is an in-memory mock tier for headless or non-Darwin platforms.
type FallbackMemoryTier struct {
	clipboard *Clipboard
}

func NewFallbackMemoryTier() *FallbackMemoryTier {
	return &FallbackMemoryTier{
		clipboard: NewClipboard(),
	}
}

func (f *FallbackMemoryTier) Name() string {
	return "fallback_memory"
}

func (f *FallbackMemoryTier) Paste(text string) error {
	_, err := f.clipboard.Paste(text)
	return err
}

// NewDefaultCascade returns the fallback cascade for non-macOS systems.
func NewDefaultCascade() *Cascade {
	return NewCascade(NewFallbackMemoryTier())
}

// GetSystemClipboard returns empty string on non-Darwin platforms.
func GetSystemClipboard() (string, error) {
	return "", nil
}

// SetSystemClipboard is a no-op on non-Darwin platforms.
func SetSystemClipboard(text string) error {
	return nil
}

