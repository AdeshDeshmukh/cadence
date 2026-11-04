package appcontext

import "context"

// MockDetector provides deterministic responses for testing.
type MockDetector struct {
	Info AppInfo
	Err  error
}

// NewMockDetector creates a mock detector with the given AppInfo.
func NewMockDetector(info AppInfo) *MockDetector {
	if info.Category == "" && info.Name != "" {
		info.Category = Categorize(info.Name, info.BundleID, info.WindowTitle)
	}
	return &MockDetector{Info: info}
}

func (m *MockDetector) Detect(_ context.Context) (AppInfo, error) {
	if m.Err != nil {
		return AppInfo{}, m.Err
	}
	return m.Info, nil
}
