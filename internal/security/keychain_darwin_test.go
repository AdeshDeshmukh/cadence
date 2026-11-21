//go:build darwin

package security

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDarwinKeychainStore_MockRunner(t *testing.T) {
	mockData := make(map[string]string)

	mockRunner := func(ctx context.Context, stdin string, name string, args ...string) ([]byte, error) {
		if name != "security" {
			return nil, errors.New("unexpected command")
		}

		if len(args) == 0 {
			return nil, errors.New("missing arguments")
		}

		subcmd := args[0]
		switch subcmd {
		case "find-generic-password":
			// args: find-generic-password -s <service> -a <key> -w
			var key string
			for i, arg := range args {
				if arg == "-a" && i+1 < len(args) {
					key = args[i+1]
				}
			}
			if val, ok := mockData[key]; ok {
				return []byte(val + "\n"), nil
			}
			return nil, errors.New("The specified item could not be found in the keychain.")

		case "add-generic-password":
			// args: add-generic-password -U -s <service> -a <key> -w <value>
			var key, val string
			for i, arg := range args {
				if arg == "-a" && i+1 < len(args) {
					key = args[i+1]
				}
				if arg == "-w" && i+1 < len(args) {
					val = args[i+1]
				}
			}
			mockData[key] = val
			return []byte(""), nil

		case "delete-generic-password":
			// args: delete-generic-password -s <service> -a <key>
			var key string
			for i, arg := range args {
				if arg == "-a" && i+1 < len(args) {
					key = args[i+1]
				}
			}
			if _, ok := mockData[key]; ok {
				delete(mockData, key)
				return []byte(""), nil
			}
			return nil, errors.New("The specified item could not be found in the keychain.")

		default:
			return nil, errors.New("unsupported security subcmd")
		}
	}

	store := NewDarwinKeychainStoreWithRunner("io.cadence.test", mockRunner)

	// Set
	if err := store.Set("openai_api_key", "sk-darwin-secret-key"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	val, err := store.Get("openai_api_key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "sk-darwin-secret-key" {
		t.Fatalf("expected sk-darwin-secret-key, got %s", val)
	}

	// List
	keys, err := store.List()
	if err != nil || len(keys) != 1 || keys[0] != "openai_api_key" {
		t.Fatalf("unexpected keys: %v, err: %v", keys, err)
	}

	// Delete
	if err := store.Delete("openai_api_key"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify not found after delete
	_, err = store.Get("openai_api_key")
	if !errors.Is(err, ErrSecretNotFound) && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}
