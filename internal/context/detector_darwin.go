//go:build darwin

package appcontext

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DarwinDetector detects active foreground applications using AppleScript and macOS Accessibility.
type DarwinDetector struct{}

// NewDefaultDetector returns the platform-specific default detector.
func NewDefaultDetector() Detector {
	return &DarwinDetector{}
}

func (d *DarwinDetector) Detect(ctx context.Context) (AppInfo, error) {
	callCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()

	script := `
tell application "System Events"
	set frontApp to first application process whose frontmost is true
	set appName to name of frontApp
	set appPID to unix id of frontApp
	set winTitle to ""
	try
		set winTitle to name of front window of frontApp
	end try
	set bundleID to ""
	try
		set bundleID to bundle identifier of frontApp
	end try
	return appName & "|||" & (appPID as string) & "|||" & winTitle & "|||" & bundleID
end tell
`
	cmd := exec.CommandContext(callCtx, "osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return AppInfo{Name: "Unknown", Category: CategoryGeneral}, fmt.Errorf("detect active app: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(out)), "|||")
	appName := ""
	pid := 0
	winTitle := ""
	bundleID := ""

	if len(parts) > 0 {
		appName = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		pid, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	}
	if len(parts) > 2 {
		winTitle = strings.TrimSpace(parts[2])
	}
	if len(parts) > 3 {
		bundleID = strings.TrimSpace(parts[3])
	}

	cat := Categorize(appName, bundleID, winTitle)

	return AppInfo{
		Name:        appName,
		BundleID:    bundleID,
		WindowTitle: winTitle,
		ProcessID:   pid,
		Category:    cat,
	}, nil
}
