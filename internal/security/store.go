package security

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var (
	// ErrSecretNotFound is returned when the requested secret does not exist in the store.
	ErrSecretNotFound = errors.New("security: secret not found")

	// ErrEmptyKey is returned when an operation is performed with an empty secret key.
	ErrEmptyKey = errors.New("security: secret key cannot be empty")

	// ErrKeychainUnavailable is returned when the system keychain service cannot be reached.
	ErrKeychainUnavailable = errors.New("security: system keychain is unavailable")
)

const (
	// DefaultServiceName is the default application namespace for secrets in the system keychain.
	DefaultServiceName = "io.cadence.secrets"

	// Standard secret key names used across Cadence
	KeyOpenAI        = "openai_api_key"
	KeyAnthropic     = "anthropic_api_key"
	KeyCustomLLM     = "custom_llm_api_key"
	KeyWebhookSecret = "webhook_secret"
)

// SecretStore provides a common interface for reading, writing, and removing secrets.
type SecretStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
	List() ([]string, error)
}

// CommandRunner allows executing system commands (mockable for unit testing).
type CommandRunner func(ctx context.Context, stdin string, name string, args ...string) ([]byte, error)

func defaultCommandRunner(ctx context.Context, stdin string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return cmd.Output()
}

// MemoryStore is an in-memory thread-safe implementation of SecretStore.
type MemoryStore struct {
	mu      sync.RWMutex
	secrets map[string]string
}

// NewMemoryStore creates a new in-memory SecretStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		secrets: make(map[string]string),
	}
}

func (m *MemoryStore) Get(key string) (string, error) {
	if key == "" {
		return "", ErrEmptyKey
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	val, ok := m.secrets[key]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, key)
	}
	return val, nil
}

func (m *MemoryStore) Set(key, value string) error {
	if key == "" {
		return ErrEmptyKey
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.secrets[key] = value
	return nil
}

func (m *MemoryStore) Delete(key string) error {
	if key == "" {
		return ErrEmptyKey
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.secrets[key]; !ok {
		return fmt.Errorf("%w: %s", ErrSecretNotFound, key)
	}
	delete(m.secrets, key)
	return nil
}

func (m *MemoryStore) List() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.secrets))
	for k := range m.secrets {
		keys = append(keys, k)
	}
	return keys, nil
}

// CascadingSecretStore checks a primary store (e.g. system keychain) and falls back to a secondary store.
type CascadingSecretStore struct {
	primary  SecretStore
	fallback SecretStore
}

// NewCascadingSecretStore creates a new cascading store.
func NewCascadingSecretStore(primary, fallback SecretStore) *CascadingSecretStore {
	if fallback == nil {
		fallback = NewMemoryStore()
	}
	return &CascadingSecretStore{
		primary:  primary,
		fallback: fallback,
	}
}

func (c *CascadingSecretStore) Get(key string) (string, error) {
	if key == "" {
		return "", ErrEmptyKey
	}
	if c.primary != nil {
		val, err := c.primary.Get(key)
		if err == nil {
			return val, nil
		}
	}
	return c.fallback.Get(key)
}

func (c *CascadingSecretStore) Set(key, value string) error {
	if key == "" {
		return ErrEmptyKey
	}
	var primaryErr error
	if c.primary != nil {
		primaryErr = c.primary.Set(key, value)
	}
	fallbackErr := c.fallback.Set(key, value)

	if primaryErr != nil && fallbackErr != nil {
		return fmt.Errorf("failed to save secret to primary (%v) and fallback (%v)", primaryErr, fallbackErr)
	}
	return nil
}

func (c *CascadingSecretStore) Delete(key string) error {
	if key == "" {
		return ErrEmptyKey
	}
	var primaryErr error
	if c.primary != nil {
		primaryErr = c.primary.Delete(key)
	}
	fallbackErr := c.fallback.Delete(key)

	if primaryErr != nil && fallbackErr != nil && errors.Is(fallbackErr, ErrSecretNotFound) {
		return ErrSecretNotFound
	}
	return nil
}

func (c *CascadingSecretStore) List() ([]string, error) {
	keySet := make(map[string]struct{})
	if c.primary != nil {
		if keys, err := c.primary.List(); err == nil {
			for _, k := range keys {
				keySet[k] = struct{}{}
			}
		}
	}
	if keys, err := c.fallback.List(); err == nil {
		for _, k := range keys {
			keySet[k] = struct{}{}
		}
	}

	result := make([]string, 0, len(keySet))
	for k := range keySet {
		result = append(result, k)
	}
	return result, nil
}

var (
	defaultStoreMu sync.RWMutex
	defaultStore   SecretStore
)

// SetDefaultSecretStore overrides the global default secret store (useful for tests or custom backends).
func SetDefaultSecretStore(s SecretStore) {
	defaultStoreMu.Lock()
	defer defaultStoreMu.Unlock()
	defaultStore = s
}

// NewDefaultSecretStore returns the platform-specific keychain store singleton.
func NewDefaultSecretStore() SecretStore {
	defaultStoreMu.Lock()
	defer defaultStoreMu.Unlock()
	if defaultStore == nil {
		defaultStore = newPlatformSecretStore()
	}
	return defaultStore
}

// ResolveSecret determines a secret value by checking:
// 1. configVal (if non-empty)
// 2. Environment variables (in order provided)
// 3. System secret store (NewDefaultSecretStore())
func ResolveSecret(key, configVal string, envVars ...string) string {
	if strings.TrimSpace(configVal) != "" {
		return strings.TrimSpace(configVal)
	}

	for _, ev := range envVars {
		if val := strings.TrimSpace(os.Getenv(ev)); val != "" {
			return val
		}
	}

	store := NewDefaultSecretStore()
	if val, err := store.Get(key); err == nil && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}

	return ""
}

// discardCloser wraps an io.Reader to ignore close calls.
type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }
