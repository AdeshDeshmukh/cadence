package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifier_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_model.bin")

	content := []byte("cadence-voice-model-payload-v1.0-clean-weights")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	hasher := sha256.New()
	hasher.Write(content)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	res, err := VerifyFile(filePath, expectedHash, int64(len(content)))
	if err != nil {
		t.Fatalf("expected verification success, got: %v", err)
	}
	if res.Status != StatusVerified {
		t.Errorf("expected StatusVerified, got: %s", res.Status)
	}
	if res.ActualSHA256 != expectedHash {
		t.Errorf("expected %s, got %s", expectedHash, res.ActualSHA256)
	}
}

func TestVerifier_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "corrupted_model.bin")

	content := []byte("original-weights-data-1234567890")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Deliberately wrong SHA256 (tampered file)
	tamperedHash := "0000000000000000000000000000000000000000000000000000000000000000"

	res, err := VerifyFile(filePath, tamperedHash, int64(len(content)))
	if err == nil {
		t.Fatal("expected checksum error, got nil")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("expected ErrChecksumMismatch, got: %v", err)
	}
	if res.Status != StatusCorrupted {
		t.Errorf("expected StatusCorrupted, got: %s", res.Status)
	}
}

func TestVerifier_SizeMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "truncated_model.bin")

	content := []byte("truncated-data")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Expect larger size than actual
	res, err := VerifyFile(filePath, "any-hash", int64(len(content)+50))
	if err == nil {
		t.Fatal("expected size mismatch error, got nil")
	}
	if !errors.Is(err, ErrSizeMismatch) {
		t.Errorf("expected ErrSizeMismatch, got: %v", err)
	}
	if res.Status != StatusSizeMismatch {
		t.Errorf("expected StatusSizeMismatch, got: %s", res.Status)
	}
}

func TestVerifier_MissingFile(t *testing.T) {
	res, err := VerifyFile("/path/to/nonexistent/cadence_test_file.bin", "somehash", 100)
	if err == nil {
		t.Fatal("expected missing error, got nil")
	}
	if !errors.Is(err, ErrModelMissing) {
		t.Errorf("expected ErrModelMissing, got: %v", err)
	}
	if res.Status != StatusMissing {
		t.Errorf("expected StatusMissing, got: %s", res.Status)
	}
}

func TestVerifier_ModelCatalogIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	content := []byte("mock-silero-vad-neural-network-weights")
	hasher := sha256.New()
	hasher.Write(content)
	computedHash := hex.EncodeToString(hasher.Sum(nil))

	testModelID := "test-catalog-model"
	regErr := RegisterModel(ModelInfo{
		ID:           testModelID,
		Name:         "Test Catalog Model",
		Engine:       "whisper",
		Architecture: "ggml",
		Filename:     "test_catalog.bin",
		Size:         int64(len(content)),
		SHA256:       computedHash,
	})
	if regErr != nil {
		t.Fatalf("register: %v", regErr)
	}

	// Missing in modelsDir
	res, err := VerifyModel(testModelID, tmpDir)
	if err == nil || res.Status != StatusMissing {
		t.Errorf("expected missing status, got: %v, err: %v", res, err)
	}

	// Create valid file
	targetFile := filepath.Join(tmpDir, "test_catalog.bin")
	if err := os.WriteFile(targetFile, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Now verified
	res, err = VerifyModel(testModelID, tmpDir)
	if err != nil {
		t.Fatalf("expected verification success, got: %v", err)
	}
	if res.Status != StatusVerified {
		t.Errorf("expected StatusVerified, got: %s", res.Status)
	}

	// Corrupt file
	corrupted := append(content, []byte("-bad-bytes")...)
	if err := os.WriteFile(targetFile, corrupted, 0644); err != nil {
		t.Fatalf("write corrupted: %v", err)
	}

	res, err = VerifyModel(testModelID, tmpDir)
	if err == nil || (res.Status != StatusCorrupted && res.Status != StatusSizeMismatch) {
		t.Errorf("expected corruption or size mismatch error, got: %v, err: %v", res, err)
	}
}
