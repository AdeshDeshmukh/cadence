package appcontext

import (
	"context"
	"errors"
	"testing"
)

func TestCategorize(t *testing.T) {
	tests := []struct {
		name        string
		bundleID    string
		windowTitle string
		expected    Category
	}{
		{"Visual Studio Code", "com.microsoft.VSCode", "cadence - main.go", CategoryCode},
		{"Cursor", "com.todesktop.230313mzl4w4u92", "app.go", CategoryCode},
		{"Xcode", "com.apple.dt.Xcode", "Cadence.xcodeproj", CategoryCode},
		{"iTerm2", "com.googlecode.iterm2", "zsh 80x24", CategoryTerminal},
		{"Terminal", "com.apple.Terminal", "bash", CategoryTerminal},
		{"Slack", "com.tinyspeck.slackmacgap", "#general - Acme Corp", CategoryChat},
		{"Discord", "com.hnc.Discord", "Server Chat", CategoryChat},
		{"Google Chrome", "com.google.Chrome", "GitHub - Issues", CategoryBrowser},
		{"Safari", "com.apple.Safari", "Documentation", CategoryBrowser},
		{"Notion", "notion.id", "Product Roadmap", CategoryDocument},
		{"Microsoft Word", "com.microsoft.Word", "Report.docx", CategoryDocument},
		{"Epic Hyperspace", "com.epic.hyperspace", "Patient Chart", CategoryMedical},
		{"Clio Manage", "com.clio.manage", "Matter #1042", CategoryLegal},
		{"Calculator", "com.apple.calculator", "Calculator", CategoryGeneral},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Categorize(tc.name, tc.bundleID, tc.windowTitle)
			if got != tc.expected {
				t.Errorf("expected category %q for app %q, got %q", tc.expected, tc.name, got)
			}
		})
	}
}

func TestMockDetector(t *testing.T) {
	expected := AppInfo{
		Name:        "Slack",
		BundleID:    "com.tinyspeck.slackmacgap",
		WindowTitle: "#engineering",
		ProcessID:   1234,
	}

	mock := NewMockDetector(expected)
	if mock.Info.Category != CategoryChat {
		t.Fatalf("expected CategoryChat, got %s", mock.Info.Category)
	}

	info, err := mock.Detect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "Slack" || info.Category != CategoryChat {
		t.Fatalf("unexpected info: %+v", info)
	}

	// Test error injection
	mock.Err = errors.New("detection failed")
	_, err = mock.Detect(context.Background())
	if err == nil {
		t.Fatalf("expected error from mock, got nil")
	}
}

func TestDefaultDetector(t *testing.T) {
	d := NewDefaultDetector()
	if d == nil {
		t.Fatalf("expected non-nil default detector")
	}

	// Calling Detect shouldn't panic
	info, err := d.Detect(context.Background())
	if err != nil {
		t.Logf("detector returned expected error in sandbox/headless environment: %v", err)
	} else {
		t.Logf("detected app: %s (%s)", info.Name, info.Category)
	}
}
