//go:build !darwin

package appcontext

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// FallbackDetector detects active applications on Linux and Windows platforms.
type FallbackDetector struct{}

// NewDefaultDetector returns the fallback platform detector.
func NewDefaultDetector() Detector {
	return &FallbackDetector{}
}

func (d *FallbackDetector) Detect(ctx context.Context) (AppInfo, error) {
	callCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// Try xdotool on Linux / X11
	if out, err := exec.CommandContext(callCtx, "xdotool", "getactivewindow", "getwindowname").Output(); err == nil {
		title := strings.TrimSpace(string(out))
		cat := Categorize(title, "", title)
		return AppInfo{
			Name:        title,
			WindowTitle: title,
			Category:    cat,
		}, nil
	}

	return AppInfo{
		Name:     "Desktop",
		Category: CategoryGeneral,
	}, nil
}
