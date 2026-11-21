package model

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestManager_ListAndVerifyAll(t *testing.T) {
	tmpDir := t.TempDir()

	content := []byte("mock-manager-weights-data")
	hasher := sha256.New()
	hasher.Write(content)
	validHash := hex.EncodeToString(hasher.Sum(nil))

	modelID := "mock-manager-model"
	_ = RegisterModel(ModelInfo{
		ID:           modelID,
		Name:         "Mock Manager Model",
		Engine:       "whisper",
		Architecture: "ggml",
		Filename:     "manager_test.bin",
		Size:         int64(len(content)),
		SHA256:       validHash,
	})

	// Before file exists
	list, err := ListModels(tmpDir)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	var found *ModelStatus
	for i := range list {
		if list[i].Model.ID == modelID {
			found = &list[i]
			break
		}
	}
	if found == nil || found.Installed || found.Status != StatusMissing {
		t.Fatalf("expected missing model before file creation, got: %v", found)
	}

	// Create valid file
	destFile := filepath.Join(tmpDir, "manager_test.bin")
	if err := os.WriteFile(destFile, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// After file exists
	list, err = ListModels(tmpDir)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	found = nil
	for i := range list {
		if list[i].Model.ID == modelID {
			found = &list[i]
			break
		}
	}
	if found == nil || !found.Installed || found.Status != StatusVerified {
		t.Fatalf("expected installed and verified model, got: %v", found)
	}

	// VerifyAll
	allResults, err := VerifyAll(tmpDir)
	if err != nil {
		t.Fatalf("VerifyAll failed: %v", err)
	}
	if len(allResults) == 0 {
		t.Fatalf("expected results from VerifyAll")
	}
}
