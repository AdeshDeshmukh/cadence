package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	// ErrModelNotFound indicates the specified model is not in the catalog.
	ErrModelNotFound = errors.New("model: not found in catalog")

	// ErrChecksumMismatch indicates a file's SHA256 does not match its expected catalog hash.
	ErrChecksumMismatch = errors.New("model: checksum verification failed (corrupted or tampered)")

	// ErrSizeMismatch indicates a file's size does not match expected catalog size.
	ErrSizeMismatch = errors.New("model: file size mismatch (incomplete download)")

	// ErrModelMissing indicates the model file does not exist on disk.
	ErrModelMissing = errors.New("model: file not found on disk")
)

// ModelInfo describes a supported model in the Cadence catalog.
type ModelInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Engine       string `json:"engine"`       // "whisper", "silero", "parakeet"
	Architecture string `json:"architecture"` // "ggml", "onnx", "nemo"
	Filename     string `json:"filename"`
	Size         int64  `json:"size"` // Expected size in bytes
	SHA256       string `json:"sha256"`
	DownloadURL  string `json:"download_url"`
	Description  string `json:"description"`
	Recommended  bool   `json:"recommended"`
}

var (
	catalogMu sync.RWMutex
	catalog   = map[string]ModelInfo{
		"whisper-tiny.en": {
			ID:           "whisper-tiny.en",
			Name:         "Whisper Tiny (English)",
			Engine:       "whisper",
			Architecture: "ggml",
			Filename:     "ggml-tiny.en.bin",
			Size:         77708534,
			SHA256:       "9644055860fa11039755455ae1b79ff8782a03cf06041a94121bc563032d8479",
			DownloadURL:  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin",
			Description:  "Fastest Whisper model, low resource usage (~390MB RAM), ideal for older hardware.",
			Recommended:  false,
		},
		"whisper-base.en": {
			ID:           "whisper-base.en",
			Name:         "Whisper Base (English)",
			Engine:       "whisper",
			Architecture: "ggml",
			Filename:     "ggml-base.en.bin",
			Size:         147964211,
			SHA256:       "137c40403d78c54018080e678a2246944e7e604ec2820d0f5080c5e7b4458f27",
			DownloadURL:  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin",
			Description:  "Default recommended balance of high speed (<200ms latency) and accurate transcription.",
			Recommended:  true,
		},
		"whisper-small.en": {
			ID:           "whisper-small.en",
			Name:         "Whisper Small (English)",
			Engine:       "whisper",
			Architecture: "ggml",
			Filename:     "ggml-small.en.bin",
			Size:         487920155,
			SHA256:       "db8a496a91d712d8a6950d371548b4d357e064a739b8be4f61e8e663047282fa",
			DownloadURL:  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.en.bin",
			Description:  "Higher accuracy for specialized technical vocabularies and diverse accents.",
			Recommended:  false,
		},
		"whisper-medium.en": {
			ID:           "whisper-medium.en",
			Name:         "Whisper Medium (English)",
			Engine:       "whisper",
			Architecture: "ggml",
			Filename:     "ggml-medium.en.bin",
			Size:         1533753195,
			SHA256:       "8c30f0e44ce956b4f6ee3e5d450058e2515c43d526177a2b3fdb06c0866f232f",
			DownloadURL:  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.en.bin",
			Description:  "Near-flawless transcription accuracy, requires GPU acceleration or modern multi-core CPU.",
			Recommended:  false,
		},
		"silero-vad": {
			ID:           "silero-vad",
			Name:         "Silero VAD (v4)",
			Engine:       "silero",
			Architecture: "onnx",
			Filename:     "silero_vad.onnx",
			Size:         1558277,
			SHA256:       "b457f920257e108bbda6ff87e9cf2efb5d5d677d242699a7008cfbf67e91eb70",
			DownloadURL:  "https://github.com/snakers4/silero-vad/raw/master/files/silero_vad.onnx",
			Description:  "Neural voice activity detection filter, processes 30ms audio windows with 99.8% precision.",
			Recommended:  true,
		},
		"parakeet-rnnt-0.6b": {
			ID:           "parakeet-rnnt-0.6b",
			Name:         "NVIDIA Parakeet-RNNT 0.6B",
			Engine:       "parakeet",
			Architecture: "nemo",
			Filename:     "parakeet_rnnt_0.6b.nemo",
			Size:         642817290,
			SHA256:       "5e9a4f61cb7041be5e52c8a149b1a51187d3e9114fef130f1d07c0809210ab56",
			DownloadURL:  "https://huggingface.co/nvidia/parakeet-rnnt-0.6b/resolve/main/parakeet-rnnt-0.6b.nemo",
			Description:  "Ultra-low latency streaming transducer model for real-time live typing.",
			Recommended:  false,
		},
	}
)

// GetCatalog returns all known models in the catalog.
func GetCatalog() []ModelInfo {
	catalogMu.RLock()
	defer catalogMu.RUnlock()

	res := make([]ModelInfo, 0, len(catalog))
	for _, m := range catalog {
		res = append(res, m)
	}
	return res
}

// LookupModel finds a model by its ID or filename.
func LookupModel(query string) (*ModelInfo, error) {
	if query == "" {
		return nil, ErrModelNotFound
	}

	catalogMu.RLock()
	defer catalogMu.RUnlock()

	normalized := strings.ToLower(strings.TrimSpace(query))

	// Direct ID match
	if info, ok := catalog[normalized]; ok {
		cp := info
		return &cp, nil
	}

	// Match by filename or prefix
	for _, m := range catalog {
		if strings.EqualFold(m.Filename, query) ||
			strings.EqualFold(m.ID, normalized) ||
			strings.EqualFold(filepath.Base(query), m.Filename) {
			cp := m
			return &cp, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrModelNotFound, query)
}

// RegisterModel adds or updates a model definition in the runtime catalog.
func RegisterModel(info ModelInfo) error {
	if info.ID == "" {
		return errors.New("model: ID cannot be empty")
	}
	if info.Filename == "" {
		return errors.New("model: Filename cannot be empty")
	}
	if info.SHA256 == "" {
		return errors.New("model: SHA256 cannot be empty")
	}

	catalogMu.Lock()
	defer catalogMu.Unlock()

	info.SHA256 = strings.ToLower(strings.TrimSpace(info.SHA256))
	catalog[strings.ToLower(info.ID)] = info
	return nil
}

// DefaultModelsDir returns the standard local models directory path.
func DefaultModelsDir() string {
	if env := os.Getenv("CADENCE_MODELS_DIR"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "cadence", "models")
	}
	return filepath.Join(home, ".local", "share", "cadence", "models")
}
