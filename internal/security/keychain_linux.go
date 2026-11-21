//go:build linux

package security

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// LinuxSecretServiceStore interacts with the Linux Secret Service API using secret-tool.
type LinuxSecretServiceStore struct {
	mu       sync.RWMutex
	service  string
	runner   CommandRunner
	fallback SecretStore
	keys     map[string]struct{}
}

var linuxSharedFallback = NewMemoryStore()

// NewLinuxSecretServiceStore creates a secret store for Linux systems.
func NewLinuxSecretServiceStore(service string) *LinuxSecretServiceStore {
	if service == "" {
		service = DefaultServiceName
	}
	return &LinuxSecretServiceStore{
		service:  service,
		runner:   defaultCommandRunner,
		fallback: linuxSharedFallback,
		keys:     make(map[string]struct{}),
	}
}

// NewLinuxSecretServiceStoreWithRunner creates a store with a custom command runner for tests.
func NewLinuxSecretServiceStoreWithRunner(service string, runner CommandRunner) *LinuxSecretServiceStore {
	if runner == nil {
		runner = defaultCommandRunner
	}
	return &LinuxSecretServiceStore{
		service:  service,
		runner:   runner,
		fallback: linuxSharedFallback,
		keys:     make(map[string]struct{}),
	}
}

func (l *LinuxSecretServiceStore) Get(key string) (string, error) {
	if key == "" {
		return "", ErrEmptyKey
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// secret-tool lookup service <service> key <key>
	out, err := l.runner(ctx, "", "secret-tool", "lookup", "service", l.service, "key", key)
	if err == nil {
		val := strings.TrimRight(string(out), "\r\n")
		if val != "" {
			return val, nil
		}
	}

	// Fallback to in-memory store in headless / CI environments
	return l.fallback.Get(key)
}

func (l *LinuxSecretServiceStore) Set(key, value string) error {
	if key == "" {
		return ErrEmptyKey
	}

	l.mu.Lock()
	l.keys[key] = struct{}{}
	l.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// secret-tool store --label="Cadence Secret" service <service> key <key>
	_, err := l.runner(ctx, value, "secret-tool", "store", "--label=Cadence Secret", "service", l.service, "key", key)
	if err != nil {
		// Fallback to in-memory store in headless / CI environments
		return l.fallback.Set(key, value)
	}

	_ = l.fallback.Set(key, value)
	return nil
}

func (l *LinuxSecretServiceStore) Delete(key string) error {
	if key == "" {
		return ErrEmptyKey
	}

	l.mu.Lock()
	delete(l.keys, key)
	l.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// secret-tool clear service <service> key <key>
	_, err := l.runner(ctx, "", "secret-tool", "clear", "service", l.service, "key", key)
	fbErr := l.fallback.Delete(key)

	if err != nil && fbErr != nil {
		return fmt.Errorf("%w: %s", ErrSecretNotFound, key)
	}

	return nil
}

func (l *LinuxSecretServiceStore) List() ([]string, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	fbKeys, _ := l.fallback.List()
	combined := make(map[string]struct{})
	for k := range l.keys {
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
	return NewLinuxSecretServiceStore(DefaultServiceName)
}
