package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Status represents the integrity state of a model file.
type Status string

const (
	// StatusVerified indicates the model file matches its exact SHA256 checksum and size.
	StatusVerified Status = "VERIFIED"

	// StatusCorrupted indicates the SHA256 hash does not match (tampered, corrupted, or bad bytes).
	StatusCorrupted Status = "CORRUPTED"

	// StatusSizeMismatch indicates the file size does not match expected length.
	StatusSizeMismatch Status = "SIZE_MISMATCH"

	// StatusMissing indicates the model file does not exist on disk.
	StatusMissing Status = "MISSING"

	// StatusUnknown indicates unverified or custom model without catalog checksum.
	StatusUnknown Status = "UNKNOWN"
)

// VerificationResult contains the detailed diagnostic output of a model integrity check.
type VerificationResult struct {
	ModelID        string `json:"model_id"`
	Path           string `json:"path"`
	ExpectedSHA256 string `json:"expected_sha256"`
	ActualSHA256   string `json:"actual_sha256"`
	ExpectedSize   int64  `json:"expected_size"`
	ActualSize     int64  `json:"actual_size"`
	Status         Status `json:"status"`
	Error          error  `json:"error,omitempty"`
}

// ComputeSHA256 computes the hex-encoded SHA-256 digest and byte size of any file.
func ComputeSHA256(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, fmt.Errorf("read file: %w", err)
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	return hash, size, nil
}

// VerifyFile inspects an arbitrary file against an expected SHA256 and optional expected size.
func VerifyFile(path string, expectedSHA256 string, expectedSize int64) (*VerificationResult, error) {
	result := &VerificationResult{
		Path:           path,
		ExpectedSHA256: strings.ToLower(strings.TrimSpace(expectedSHA256)),
		ExpectedSize:   expectedSize,
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			result.Status = StatusMissing
			result.Error = ErrModelMissing
			return result, ErrModelMissing
		}
		result.Status = StatusMissing
		result.Error = err
		return result, err
	}

	result.ActualSize = info.Size()

	// If expected size is given and does not match
	if expectedSize > 0 && result.ActualSize != expectedSize {
		result.Status = StatusSizeMismatch
		result.Error = fmt.Errorf("%w: expected %d bytes, found %d bytes", ErrSizeMismatch, expectedSize, result.ActualSize)
		return result, result.Error
	}

	// Compute SHA256
	actualHash, _, err := ComputeSHA256(path)
	if err != nil {
		result.Status = StatusCorrupted
		result.Error = err
		return result, err
	}
	result.ActualSHA256 = actualHash

	// If no expected SHA256 was supplied, we cannot verify, mark unknown
	if result.ExpectedSHA256 == "" {
		result.Status = StatusUnknown
		return result, nil
	}

	if !strings.EqualFold(result.ActualSHA256, result.ExpectedSHA256) {
		result.Status = StatusCorrupted
		result.Error = fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, result.ExpectedSHA256, result.ActualSHA256)
		return result, result.Error
	}

	result.Status = StatusVerified
	return result, nil
}

// VerifyModel looks up the model in the catalog and verifies its file in modelsDir.
func VerifyModel(idOrName string, modelsDir string) (*VerificationResult, error) {
	if modelsDir == "" {
		modelsDir = DefaultModelsDir()
	}

	info, err := LookupModel(idOrName)
	if err != nil {
		// If not in catalog, check if idOrName is a direct file path
		if _, statErr := os.Stat(idOrName); statErr == nil {
			return VerifyFile(idOrName, "", 0)
		}
		return nil, err
	}

	targetPath := filepath.Join(modelsDir, info.Filename)
	res, err := VerifyFile(targetPath, info.SHA256, info.Size)
	res.ModelID = info.ID
	return res, err
}

// AssertModelIntegrity ensures a model file is present and not corrupted before engine initialization.
func AssertModelIntegrity(path string) error {
	if path == "" {
		return errors.New("model: path is empty")
	}

	filename := filepath.Base(path)
	info, err := LookupModel(filename)
	if err != nil {
		// Not in official catalog (custom user model), just verify file exists and is readable
		if _, statErr := os.Stat(path); statErr != nil {
			return fmt.Errorf("custom model missing: %w", statErr)
		}
		return nil
	}

	_, verifyErr := VerifyFile(path, info.SHA256, info.Size)
	return verifyErr
}
