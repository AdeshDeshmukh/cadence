//go:build linux

package security

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLinuxSecretServiceStore_MockRunner(t *testing.T) {
	mockData := make(map[string]string)

	mockRunner := func(ctx context.Context, stdin string, name string, args ...string) ([]byte, error) {
		if name != "secret-tool" {
			return nil, errors.New("unexpected command")
		}

		if len(args) == 0 {
			return nil, errors.New("missing arguments")
		}

		subcmd := args[0]
		switch subcmd {
		case "lookup":
			// secret-tool lookup service <service> key <key>
			var key string
			for i, arg := range args {
				if arg == "key" && i+1 < len(args) {
					key = args[i+1]
				}
			}
			if val, ok := mockData[key]; ok {
				return []byte(val), nil
			}
			return nil, errors.New("not found")

		case "store":
			// secret-tool store --label=... service <service> key <key>
			var key string
			for i, arg := range args {
				if arg == "key" && i+1 < len(args) {
					key = args[i+1]
				}
			}
			mockData[key] = stdin
			return []byte(""), nil

		case "clear":
			// secret-tool clear service <service> key <key>
			var key string
			for i, arg := range args {
				if arg == "key" && i+1 < len(args) {
					key = args[i+1]
				}
			}
			if _, ok := mockData[key]; ok {
				delete(mockData, key)
				return []byte(""), nil
			}
			return nil, errors.New("not found")

		default:
			return nil, errors.New("unsupported secret-tool subcmd")
		}
	}

	store := NewLinuxSecretServiceStoreWithRunner("io.cadence.test", mockRunner)

	// Set
	if err := store.Set("anthropic_api_key", "sk-ant-test-key"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	val, err := store.Get("anthropic_api_key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "sk-ant-test-key" {
		t.Fatalf("expected sk-ant-test-key, got %s", val)
	}

	// List
	keys, err := store.List()
	if err != nil || len(keys) != 1 || keys[0] != "anthropic_api_key" {
		t.Fatalf("unexpected keys: %v, err: %v", keys, err)
	}

	// Delete
	if err := store.Delete("anthropic_api_key"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify not found after delete
	_, err = store.Get("anthropic_api_key")
	if !errors.Is(err, ErrSecretNotFound) && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}
