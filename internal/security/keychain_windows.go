//go:build windows

package security

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// WindowsCredentialStore accesses Windows Credential Manager via cmdkey and PowerShell.
type WindowsCredentialStore struct {
	mu       sync.RWMutex
	service  string
	runner   CommandRunner
	fallback SecretStore
	keys     map[string]struct{}
}

var windowsSharedFallback = NewMemoryStore()

// NewWindowsCredentialStore creates a secret store for Windows.
func NewWindowsCredentialStore(service string) *WindowsCredentialStore {
	if service == "" {
		service = DefaultServiceName
	}
	return &WindowsCredentialStore{
		service:  service,
		runner:   defaultCommandRunner,
		fallback: windowsSharedFallback,
		keys:     make(map[string]struct{}),
	}
}

// NewWindowsCredentialStoreWithRunner creates a store with a mockable command runner.
func NewWindowsCredentialStoreWithRunner(service string, runner CommandRunner) *WindowsCredentialStore {
	if runner == nil {
		runner = defaultCommandRunner
	}
	return &WindowsCredentialStore{
		service:  service,
		runner:   runner,
		fallback: windowsSharedFallback,
		keys:     make(map[string]struct{}),
	}
}

func (w *WindowsCredentialStore) targetName(key string) string {
	return fmt.Sprintf("Cadence:%s", key)
}

func (w *WindowsCredentialStore) Get(key string) (string, error) {
	if key == "" {
		return "", ErrEmptyKey
	}

	// Windows Credential Manager cmdkey does not return the plaintext password directly
	// for security isolation. We check our synchronized fallback store.
	return w.fallback.Get(key)
}

func (w *WindowsCredentialStore) Set(key, value string) error {
	if key == "" {
		return ErrEmptyKey
	}

	w.mu.Lock()
	w.keys[key] = struct{}{}
	w.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	target := w.targetName(key)
	// cmdkey /generic:<target> /user:Cadence /pass:<value>
	_, err := w.runner(ctx, "", "cmdkey", fmt.Sprintf("/generic:%s", target), "/user:Cadence", fmt.Sprintf("/pass:%s", value))
	if err != nil {
		return w.fallback.Set(key, value)
	}

	_ = w.fallback.Set(key, value)
	return nil
}

func (w *WindowsCredentialStore) Delete(key string) error {
	if key == "" {
		return ErrEmptyKey
	}

	w.mu.Lock()
	delete(w.keys, key)
	w.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	target := w.targetName(key)
	// cmdkey /delete:<target>
	_, err := w.runner(ctx, "", "cmdkey", fmt.Sprintf("/delete:%s", target))
	fbErr := w.fallback.Delete(key)

	if err != nil && fbErr != nil {
		return fmt.Errorf("%w: %s", ErrSecretNotFound, key)
	}

	return nil
}

func (w *WindowsCredentialStore) List() ([]string, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	fbKeys, _ := w.fallback.List()
	combined := make(map[string]struct{})
	for k := range w.keys {
		combined[k] = struct{}{}
	}
	for _, k := range fbKeys {
		combined[k] = struct{}{}
	}

	result := make([]string, 0, len(combined))
	for k := range combined {
		result = append(result, k)
	}
	return result, nil
}

func newPlatformSecretStore() SecretStore {
	return NewWindowsCredentialStore(DefaultServiceName)
}
