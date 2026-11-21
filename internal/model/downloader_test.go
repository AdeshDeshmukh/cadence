package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDownloader_SuccessAndIntegrityVerification(t *testing.T) {
	content := []byte("whisper-ggml-binary-weights-simulated-payload")
	hasher := sha256.New()
	hasher.Write(content)
	validHash := hex.EncodeToString(hasher.Sum(nil))

	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Body:          io.NopCloser(bytes.NewReader(content)),
			ContentLength: int64(len(content)),
			Header:        make(http.Header),
		}, nil
	})

	client := &http.Client{Transport: mockTransport}

	modelID := "mock-download-test-model"
	_ = RegisterModel(ModelInfo{
		ID:           modelID,
		Name:         "Mock Download Test Model",
		Engine:       "whisper",
		Architecture: "ggml",
		Filename:     "mock_download.bin",
		Size:         int64(len(content)),
		SHA256:       validHash,
		DownloadURL:  "https://example.com/mock_download.bin",
	})

	tmpDir := t.TempDir()
	dl := NewDownloader(WithHTTPClient(client))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := dl.Download(ctx, modelID, tmpDir)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if res.Status != StatusVerified {
		t.Errorf("expected StatusVerified, got: %s", res.Status)
	}

	// Verify file exists on disk
	destFile := filepath.Join(tmpDir, "mock_download.bin")
	if data, readErr := os.ReadFile(destFile); readErr != nil || string(data) != string(content) {
		t.Fatalf("expected written content to match, err: %v", readErr)
	}

	// EnsureModel should return existing path without re-downloading
	path, err := dl.EnsureModel(ctx, modelID, tmpDir)
	if err != nil {
		t.Fatalf("EnsureModel failed: %v", err)
	}
	if path != destFile {
		t.Errorf("expected path %s, got %s", destFile, path)
	}
}

func TestDownloader_ChecksumMismatchDetection(t *testing.T) {
	content := []byte("tampered-corrupted-server-bytes")

	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Body:          io.NopCloser(bytes.NewReader(content)),
			ContentLength: int64(len(content)),
			Header:        make(http.Header),
		}, nil
	})

	client := &http.Client{Transport: mockTransport}

	modelID := "mock-tampered-model"
	_ = RegisterModel(ModelInfo{
		ID:           modelID,
		Name:         "Mock Tampered Model",
		Engine:       "whisper",
		Architecture: "ggml",
		Filename:     "tampered.bin",
		Size:         int64(len(content)),
		SHA256:       "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", // Bad hash
		DownloadURL:  "https://example.com/tampered.bin",
	})

	tmpDir := t.TempDir()
	dl := NewDownloader(WithHTTPClient(client))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := dl.Download(ctx, modelID, tmpDir)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("expected ErrChecksumMismatch, got: %v", err)
	}
	if res == nil || res.Status != StatusCorrupted {
		t.Errorf("expected StatusCorrupted, got: %v", res)
	}

	// Verify partial .part file was deleted
	partPath := filepath.Join(tmpDir, "tampered.bin.part")
	if _, statErr := os.Stat(partPath); statErr == nil {
		t.Errorf("expected temp part file to be removed on failure")
	}
	// Verify final file was not created
	finalPath := filepath.Join(tmpDir, "tampered.bin")
	if _, statErr := os.Stat(finalPath); statErr == nil {
		t.Errorf("expected final file to not exist")
	}
}
