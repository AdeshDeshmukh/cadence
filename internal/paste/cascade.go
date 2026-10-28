package paste

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	// ErrAllTiersFailed is returned when every tier in the cascade fails.
	ErrAllTiersFailed = errors.New("paste: all cascade tiers failed")

	// ErrEmptyText is returned when attempting to paste an empty string.
	ErrEmptyText = errors.New("paste: text is empty")
)

// TierPaster represents a single paste mechanism in the fallback cascade.
type TierPaster interface {
	Name() string
	Paste(text string) error
}

// Cascade executes a tiered sequence of paste implementations until one succeeds.
type Cascade struct {
	mu    sync.RWMutex
	tiers []TierPaster
}

// NewCascade creates a Cascade with the specified ordered tiers.
func NewCascade(tiers ...TierPaster) *Cascade {
	return &Cascade{
		tiers: tiers,
	}
}

// AddTier appends a fallback tier to the cascade.
func (c *Cascade) AddTier(tier TierPaster) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tiers = append(c.tiers, tier)
}

// Tiers returns the names of all registered tiers in execution order.
func (c *Cascade) Tiers() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, len(c.tiers))
	for i, t := range c.tiers {
		names[i] = t.Name()
	}
	return names
}

// Paste attempts to insert text using the tiers in sequence until one succeeds.
func (c *Cascade) Paste(text string) (Result, error) {
	if text == "" {
		return Result{}, ErrEmptyText
	}

	c.mu.RLock()
	tiersCopy := make([]TierPaster, len(c.tiers))
	copy(tiersCopy, c.tiers)
	c.mu.RUnlock()

	if len(tiersCopy) == 0 {
		return Result{}, ErrAllTiersFailed
	}

	var errMessages []string

	for _, tier := range tiersCopy {
		if err := tier.Paste(text); err == nil {
			return Result{
				Method:  tier.Name(),
				Success: true,
			}, nil
		} else {
			errMessages = append(errMessages, fmt.Sprintf("%s: %v", tier.Name(), err))
		}
	}

	return Result{}, fmt.Errorf("%w: [%s]", ErrAllTiersFailed, strings.Join(errMessages, "; "))
}
