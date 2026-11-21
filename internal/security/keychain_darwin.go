//go:build darwin

package security

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DarwinKeychainStore accesses the macOS Keychain via the security command line utility.
type DarwinKeychainStore struct {
	mu       sync.RWMutex
	service  string
	runner   CommandRunner
	fallback SecretStore
	keys     map[string]struct{}
}

var darwinSharedFallback = NewMemoryStore()

// NewDarwinKeychainStore creates a new macOS Keychain secret store.
func NewDarwinKeychainStore(service string) *DarwinKeychainStore {
	if service == "" {
		service = DefaultServiceName
	}
	return &DarwinKeychainStore{
		service:  service,
		runner:   defaultCommandRunner,
		fallback: darwinSharedFallback,
		keys:     make(map[string]struct{}),
	}
}

// NewDarwinKeychainStoreWithRunner creates a store with a mockable command runner for testing.
func NewDarwinKeychainStoreWithRunner(service string, runner CommandRunner) *DarwinKeychainStore {
	if runner == nil {
		runner = defaultCommandRunner
	}
	return &DarwinKeychainStore{
		service:  service,
		runner:   runner,
		fallback: darwinSharedFallback,
		keys:     make(map[string]struct{}),
	}
}

func (d *DarwinKeychainStore) Get(key string) (string, error) {
	if key == "" {
		return "", ErrEmptyKey
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// security find-generic-password -s <service> -a <key> -w
	out, err := d.runner(ctx, "", "security", "find-generic-password", "-s", d.service, "-a", key, "-w")
	if err == nil {
		val := strings.TrimRight(string(out), "\r\n")
		return val, nil
	}

	// If keychain lookup fails (e.g. headless sandbox or item not in keychain), check fallback
	return d.fallback.Get(key)
}

func (d *DarwinKeychainStore) Set(key, value string) error {
	if key == "" {
		return ErrEmptyKey
	}

	d.mu.Lock()
	d.keys[key] = struct{}{}
	d.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// -U updates item if it already exists
	// security add-generic-password -U -s <service> -a <key> -w <value>
	_, err := d.runner(ctx, "", "security", "add-generic-password", "-U", "-s", d.service, "-a", key, "-w", value)
	if err != nil {
		// If keychain write fails (e.g. sandbox restriction), store in fallback
		return d.fallback.Set(key, value)
	}

	_ = d.fallback.Set(key, value)
	return nil
}

func (d *DarwinKeychainStore) Delete(key string) error {
	if key == "" {
		return ErrEmptyKey
	}

	d.mu.Lock()
	delete(d.keys, key)
	d.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// security delete-generic-password -s <service> -a <key>
	_, err := d.runner(ctx, "", "security", "delete-generic-password", "-s", d.service, "-a", key)
	fbErr := d.fallback.Delete(key)

	if err != nil && fbErr != nil {
		return fmt.Errorf("%w: %s", ErrSecretNotFound, key)
	}

	return nil
}

func (d *DarwinKeychainStore) List() ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	fbKeys, _ := d.fallback.List()
	combined := make(map[string]struct{})
	for k := range d.keys {
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
	return NewDarwinKeychainStore(DefaultServiceName)
}
