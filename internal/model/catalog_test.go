package model

import (
	"testing"
)

func TestCatalog_GetAndLookup(t *testing.T) {
	catalog := GetCatalog()
	if len(catalog) == 0 {
		t.Fatal("expected non-empty model catalog")
	}

	// Lookup by exact ID
	m, err := LookupModel("whisper-base.en")
	if err != nil {
		t.Fatalf("failed to lookup whisper-base.en: %v", err)
	}
	if m.Filename != "ggml-base.en.bin" {
		t.Errorf("unexpected filename: %s", m.Filename)
	}
	if m.SHA256 == "" {
		t.Errorf("expected non-empty SHA256")
	}

	// Lookup by filename
	m2, err := LookupModel("ggml-base.en.bin")
	if err != nil {
		t.Fatalf("failed to lookup by filename: %v", err)
	}
	if m2.ID != "whisper-base.en" {
		t.Errorf("expected ID whisper-base.en, got: %s", m2.ID)
	}

	// Lookup non-existent
	_, err = LookupModel("non-existent-model-xyz")
	if err == nil {
		t.Errorf("expected error for non-existent model")
	}
}

func TestCatalog_RegisterModel(t *testing.T) {
	custom := ModelInfo{
		ID:           "custom-test-model",
		Name:         "Custom Test Model",
		Engine:       "whisper",
		Architecture: "ggml",
		Filename:     "custom.bin",
		Size:         1024,
		SHA256:       "E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855",
	}

	if err := RegisterModel(custom); err != nil {
		t.Fatalf("failed to register custom model: %v", err)
	}

	looked, err := LookupModel("custom-test-model")
	if err != nil {
		t.Fatalf("failed to lookup registered model: %v", err)
	}
	// Case normalization check
	if looked.SHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("expected lowercased SHA256, got: %s", looked.SHA256)
	}
}
