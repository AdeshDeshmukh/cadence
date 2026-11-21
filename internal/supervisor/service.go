package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ServiceStatus describes the system service registration state of cadenced.
type ServiceStatus struct {
	Platform    string `json:"platform"`
	ServiceType string `json:"service_type"` // "launchd", "systemd", "windows"
	Installed   bool   `json:"installed"`
	Running     bool   `json:"running"`
	PID         int    `json:"pid,omitempty"`
	FilePath    string `json:"file_path"`
}

// ServiceManager abstracts OS-level service initialization and lifecycle.
type ServiceManager interface {
	Install(execPath string) error
	Uninstall() error
	Status() (ServiceStatus, error)
	ServiceFilePath() string
	Definition(execPath string) string
}

// NewServiceManager returns the platform-specific service manager.
func NewServiceManager() ServiceManager {
	switch runtime.GOOS {
	case "darwin":
		return NewDarwinLaunchdService()
	case "linux":
		return NewLinuxSystemdService()
	default:
		return NewGenericService()
	}
}

// --- macOS launchd ---

type DarwinLaunchdService struct{}

func NewDarwinLaunchdService() *DarwinLaunchdService {
	return &DarwinLaunchdService{}
}

func (d *DarwinLaunchdService) ServiceFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "io.cadence.cadenced.plist")
}

func (d *DarwinLaunchdService) Definition(execPath string) string {
	if execPath == "" {
		execPath = "/usr/local/bin/cadenced"
	}
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, "Library", "Logs", "cadence", "cadenced.log")

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>io.cadence.cadenced</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>KeepAlive</key>
    <true/>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
    <key>ProcessType</key>
    <string>Interactive</string>
</dict>
</plist>
`, execPath, logPath, logPath)
}

func (d *DarwinLaunchdService) Install(execPath string) error {
	filePath := d.ServiceFilePath()
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, "Library", "Logs", "cadence")
	_ = os.MkdirAll(logDir, 0755)

	content := d.Definition(execPath)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return err
	}

	// Load into launchd
	_ = exec.Command("launchctl", "unload", filePath).Run()
	_ = exec.Command("launchctl", "load", filePath).Run()
	return nil
}

func (d *DarwinLaunchdService) Uninstall() error {
	filePath := d.ServiceFilePath()
	_ = exec.Command("launchctl", "unload", filePath).Run()
	return os.Remove(filePath)
}

func (d *DarwinLaunchdService) Status() (ServiceStatus, error) {
	filePath := d.ServiceFilePath()
	_, err := os.Stat(filePath)
	installed := err == nil

	st := ServiceStatus{
		Platform:    "darwin",
		ServiceType: "launchd",
		Installed:   installed,
		FilePath:    filePath,
	}

	if installed {
		out, err := exec.Command("launchctl", "list", "io.cadence.cadenced").CombinedOutput()
		if err == nil && strings.Contains(string(out), "io.cadence.cadenced") {
			st.Running = true
		}
	}

	return st, nil
}

// --- Linux systemd ---

type LinuxSystemdService struct{}

func NewLinuxSystemdService() *LinuxSystemdService {
	return &LinuxSystemdService{}
}

func (l *LinuxSystemdService) ServiceFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "cadenced.service")
}

func (l *LinuxSystemdService) Definition(execPath string) string {
	if execPath == "" {
		execPath = "/usr/local/bin/cadenced"
	}
	return fmt.Sprintf(`[Unit]
Description=Cadence AI Voice Dictation Daemon
After=sound.target network.target

[Service]
Type=simple
ExecStart=%s
Restart=always
RestartSec=2
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
`, execPath)
}

func (l *LinuxSystemdService) Install(execPath string) error {
	filePath := l.ServiceFilePath()
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	content := l.Definition(execPath)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return err
	}

	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	_ = exec.Command("systemctl", "--user", "enable", "cadenced").Run()
	_ = exec.Command("systemctl", "--user", "restart", "cadenced").Run()
	return nil
}

func (l *LinuxSystemdService) Uninstall() error {
	_ = exec.Command("systemctl", "--user", "stop", "cadenced").Run()
	_ = exec.Command("systemctl", "--user", "disable", "cadenced").Run()
	filePath := l.ServiceFilePath()
	err := os.Remove(filePath)
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return err
}

func (l *LinuxSystemdService) Status() (ServiceStatus, error) {
	filePath := l.ServiceFilePath()
	_, err := os.Stat(filePath)
	installed := err == nil

	st := ServiceStatus{
		Platform:    "linux",
		ServiceType: "systemd",
		Installed:   installed,
		FilePath:    filePath,
	}

	if installed {
		out, err := exec.Command("systemctl", "--user", "is-active", "cadenced").Output()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			st.Running = true
		}
	}

	return st, nil
}

// --- Generic / Fallback ---

type GenericService struct{}

func NewGenericService() *GenericService {
	return &GenericService{}
}

func (g *GenericService) ServiceFilePath() string {
	return filepath.Join(os.TempDir(), "cadenced.service")
}

func (g *GenericService) Definition(execPath string) string {
	return fmt.Sprintf("# Cadence service definition for %s\nexec %s\n", runtime.GOOS, execPath)
}

func (g *GenericService) Install(execPath string) error {
	return os.WriteFile(g.ServiceFilePath(), []byte(g.Definition(execPath)), 0644)
}

func (g *GenericService) Uninstall() error {
	return os.Remove(g.ServiceFilePath())
}

func (g *GenericService) Status() (ServiceStatus, error) {
	_, err := os.Stat(g.ServiceFilePath())
	return ServiceStatus{
		Platform:    runtime.GOOS,
		ServiceType: "generic",
		Installed:   err == nil,
		FilePath:    g.ServiceFilePath(),
	}, nil
}
