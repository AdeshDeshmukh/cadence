package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProgressFunc receives progress updates (bytesRead, totalBytes).
type ProgressFunc func(bytesRead, totalBytes int64)

// Downloader manages model downloads with real-time SHA-256 verification.
type Downloader struct {
	client     *http.Client
	onProgress ProgressFunc
}

// DownloaderOption configures Downloader.
type DownloaderOption func(*Downloader)

// WithHTTPClient customizes the HTTP client.
func WithHTTPClient(client *http.Client) DownloaderOption {
	return func(d *Downloader) {
		if client != nil {
			d.client = client
		}
	}
}

// WithProgressCallback attaches a download progress callback.
func WithProgressCallback(fn ProgressFunc) DownloaderOption {
	return func(d *Downloader) {
		d.onProgress = fn
	}
}

// NewDownloader creates a new Downloader instance.
func NewDownloader(opts ...DownloaderOption) *Downloader {
	d := &Downloader{
		client: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Download downloads a model from catalog and performs real-time SHA-256 verification.
func (d *Downloader) Download(ctx context.Context, modelID string, destDir string) (*VerificationResult, error) {
	info, err := LookupModel(modelID)
	if err != nil {
		return nil, err
	}

	if destDir == "" {
		destDir = DefaultModelsDir()
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("create models directory: %w", err)
	}

	finalPath := filepath.Join(destDir, info.Filename)
	partPath := finalPath + ".part"

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.DownloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download server returned HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Prepare output temporary file
	out, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		out.Close()
		// Clean up partial file on failure
		if _, statErr := os.Stat(partPath); statErr == nil {
			_ = os.Remove(partPath)
		}
	}()

	hasher := sha256.New()
	writer := io.MultiWriter(out, hasher)

	totalBytes := resp.ContentLength
	if totalBytes <= 0 && info.Size > 0 {
		totalBytes = info.Size
	}

	buf := make([]byte, 64*1024)
	var bytesRead int64

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := writer.Write(buf[:n]); writeErr != nil {
				return nil, fmt.Errorf("write bytes: %w", writeErr)
			}
			bytesRead += int64(n)
			if d.onProgress != nil {
				d.onProgress(bytesRead, totalBytes)
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, fmt.Errorf("read response stream: %w", readErr)
		}
	}

	_ = out.Close()

	// Verify size if expected
	if info.Size > 0 && bytesRead != info.Size {
		return &VerificationResult{
			ModelID:      info.ID,
			Path:         finalPath,
			ExpectedSize: info.Size,
			ActualSize:   bytesRead,
			Status:       StatusSizeMismatch,
			Error:        ErrSizeMismatch,
		}, ErrSizeMismatch
	}

	// Verify SHA-256
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHash, info.SHA256) {
		return &VerificationResult{
			ModelID:        info.ID,
			Path:           finalPath,
			ExpectedSHA256: info.SHA256,
			ActualSHA256:   actualHash,
			ExpectedSize:   info.Size,
			ActualSize:     bytesRead,
			Status:         StatusCorrupted,
			Error:          ErrChecksumMismatch,
		}, ErrChecksumMismatch
	}

	// Atomically move verified part file to final path
	if err := os.Rename(partPath, finalPath); err != nil {
		return nil, fmt.Errorf("save verified model: %w", err)
	}

	return &VerificationResult{
		ModelID:        info.ID,
		Path:           finalPath,
		ExpectedSHA256: info.SHA256,
		ActualSHA256:   actualHash,
		ExpectedSize:   info.Size,
		ActualSize:     bytesRead,
		Status:         StatusVerified,
	}, nil
}

// EnsureModel checks if a model exists and is verified; if missing or corrupted, downloads it.
func (d *Downloader) EnsureModel(ctx context.Context, modelID string, destDir string) (string, error) {
	if destDir == "" {
		destDir = DefaultModelsDir()
	}

	info, err := LookupModel(modelID)
	if err != nil {
		return "", err
	}

	finalPath := filepath.Join(destDir, info.Filename)
	res, _ := VerifyFile(finalPath, info.SHA256, info.Size)
	if res != nil && res.Status == StatusVerified {
		return finalPath, nil
	}

	// File missing or corrupted -> download with verification
	downloadRes, err := d.Download(ctx, modelID, destDir)
	if err != nil {
		return "", fmt.Errorf("download and verify model: %w", err)
	}

	return downloadRes.Path, nil
}
