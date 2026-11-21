package security

import (
	"errors"
	"os"
	"sort"
	"testing"
)

func TestMemoryStore_CRUD(t *testing.T) {
	s := NewMemoryStore()

	// Initial empty key check
	if _, err := s.Get(""); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("expected ErrEmptyKey, got %v", err)
	}
	if err := s.Set("", "val"); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("expected ErrEmptyKey, got %v", err)
	}
	if err := s.Delete(""); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("expected ErrEmptyKey, got %v", err)
	}

	// Not found check
	if _, err := s.Get("nonexistent"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}

	// Set and Get
	if err := s.Set("api_key", "sk-test-12345"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, err := s.Get("api_key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "sk-test-12345" {
		t.Fatalf("expected sk-test-12345, got %s", val)
	}

	// List
	_ = s.Set("second_key", "secret-2")
	keys, err := s.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	sort.Strings(keys)
	if keys[0] != "api_key" || keys[1] != "second_key" {
		t.Fatalf("unexpected keys: %v", keys)
	}

	// Delete
	if err := s.Delete("api_key"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := s.Get("api_key"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound after deletion, got %v", err)
	}

	// Delete non-existent
	if err := s.Delete("api_key"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound on double delete, got %v", err)
	}
}

func TestCascadingSecretStore(t *testing.T) {
	primary := NewMemoryStore()
	fallback := NewMemoryStore()
	cascade := NewCascadingSecretStore(primary, fallback)

	// Set on primary directly
	_ = primary.Set("k1", "v1")
	// Set on fallback directly
	_ = fallback.Set("k2", "v2")

	// Get k1 from primary
	if v, err := cascade.Get("k1"); err != nil || v != "v1" {
		t.Fatalf("expected v1 from primary, got %s (err: %v)", v, err)
	}

	// Get k2 from fallback
	if v, err := cascade.Get("k2"); err != nil || v != "v2" {
		t.Fatalf("expected v2 from fallback, got %s (err: %v)", v, err)
	}

	// Set via cascade (propagates to fallback)
	if err := cascade.Set("k3", "v3"); err != nil {
		t.Fatalf("cascade Set failed: %v", err)
	}
	if v, err := cascade.Get("k3"); err != nil || v != "v3" {
		t.Fatalf("expected v3, got %s", v)
	}

	// List across both
	keys, err := cascade.List()
	if err != nil {
		t.Fatalf("cascade List failed: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(keys), keys)
	}

	// Delete k1
	if err := cascade.Delete("k1"); err != nil {
		t.Fatalf("cascade Delete failed: %v", err)
	}
}

func TestResolveSecret_Precedence(t *testing.T) {
	// Case 1: configVal has highest precedence
	res1 := ResolveSecret("openai_api_key", "sk-config-value", "TEST_OPENAI_ENV")
	if res1 != "sk-config-value" {
		t.Fatalf("expected config value to take precedence, got %s", res1)
	}

	// Case 2: environment variable takes precedence over empty config
	_ = os.Setenv("CADENCE_TEST_SECRET_ENV", "sk-env-value")
	defer func() { _ = os.Unsetenv("CADENCE_TEST_SECRET_ENV") }()

	res2 := ResolveSecret("openai_api_key", "", "CADENCE_TEST_SECRET_ENV")
	if res2 != "sk-env-value" {
		t.Fatalf("expected env value to take precedence over store, got %s", res2)
	}

	// Case 3: store fallback when config and env are empty
	res3 := ResolveSecret("unknown_key_xyz", "", "NON_EXISTENT_ENV_123")
	if res3 != "" {
		t.Fatalf("expected empty string when not found, got %s", res3)
	}
}
