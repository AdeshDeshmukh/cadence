package model

import (
	"path/filepath"
)

// ModelStatus represents the local installation and integrity status of a catalog model.
type ModelStatus struct {
	Model        ModelInfo `json:"model"`
	Installed    bool      `json:"installed"`
	Status       Status    `json:"status"`
	LocalPath    string    `json:"local_path,omitempty"`
	ActualSize   int64     `json:"actual_size,omitempty"`
	ActualSHA256 string    `json:"actual_sha256,omitempty"`
	Error        string    `json:"error,omitempty"`
}

// ListModels scans modelsDir and correlates with the catalog.
func ListModels(modelsDir string) ([]ModelStatus, error) {
	if modelsDir == "" {
		modelsDir = DefaultModelsDir()
	}

	catalogList := GetCatalog()
	statuses := make([]ModelStatus, 0, len(catalogList))

	for _, m := range catalogList {
		localPath := filepath.Join(modelsDir, m.Filename)
		res, err := VerifyFile(localPath, m.SHA256, m.Size)

		st := ModelStatus{
			Model:     m,
			LocalPath: localPath,
		}

		if res != nil {
			st.Status = res.Status
			st.ActualSize = res.ActualSize
			st.ActualSHA256 = res.ActualSHA256
			st.Installed = (res.Status == StatusVerified || res.Status == StatusCorrupted || res.Status == StatusSizeMismatch)
			if err != nil {
				st.Error = err.Error()
			}
		} else {
			st.Status = StatusMissing
		}

		statuses = append(statuses, st)
	}

	return statuses, nil
}

// VerifyAll checks the cryptographic integrity of all installed models in modelsDir.
func VerifyAll(modelsDir string) ([]VerificationResult, error) {
	if modelsDir == "" {
		modelsDir = DefaultModelsDir()
	}

	catalogList := GetCatalog()
	var results []VerificationResult

	for _, m := range catalogList {
		localPath := filepath.Join(modelsDir, m.Filename)
		res, _ := VerifyFile(localPath, m.SHA256, m.Size)
		if res != nil {
			res.ModelID = m.ID
			results = append(results, *res)
		}
	}

	return results, nil
}
