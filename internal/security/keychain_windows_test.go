//go:build windows

package security

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWindowsCredentialStore_MockRunner(t *testing.T) {
	mockData := make(map[string]string)

	mockRunner := func(ctx context.Context, stdin string, name string, args ...string) ([]byte, error) {
		if name != "cmdkey" {
			return nil, errors.New("unexpected command")
		}

		for _, arg := range args {
			if strings.HasPrefix(arg, "/generic:") {
				target := strings.TrimPrefix(arg, "/generic:")
				key := strings.TrimPrefix(target, "Cadence:")
				for _, p := range args {
					if strings.HasPrefix(p, "/pass:") {
						mockData[key] = strings.TrimPrefix(p, "/pass:")
					}
				}
				return []byte("CMDKEY: Credential added successfully.\n"), nil
			}
			if strings.HasPrefix(arg, "/delete:") {
				target := strings.TrimPrefix(arg, "/delete:")
				key := strings.TrimPrefix(target, "Cadence:")
				if _, ok := mockData[key]; ok {
					delete(mockData, key)
					return []byte("CMDKEY: Credential deleted successfully.\n"), nil
				}
				return nil, errors.New("Element not found")
			}
		}
		return nil, errors.New("unsupported cmdkey arguments")
	}

	store := NewWindowsCredentialStoreWithRunner("io.cadence.test", mockRunner)

	// Set
	if err := store.Set("webhook_secret", "whsec-test-windows"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	val, err := store.Get("webhook_secret")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "whsec-test-windows" {
		t.Fatalf("expected whsec-test-windows, got %s", val)
	}

	// List
	keys, err := store.List()
	if err != nil || len(keys) != 1 || keys[0] != "webhook_secret" {
		t.Fatalf("unexpected keys: %v, err: %v", keys, err)
	}

	// Delete
	if err := store.Delete("webhook_secret"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify not found after delete
	_, err = store.Get("webhook_secret")
	if !errors.Is(err, ErrSecretNotFound) && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}
