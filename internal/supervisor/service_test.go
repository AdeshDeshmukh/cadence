package supervisor

import (
	"strings"
	"testing"
)

func TestDarwinLaunchdService_Definition(t *testing.T) {
	d := NewDarwinLaunchdService()
	def := d.Definition("/usr/local/bin/cadenced")

	if !strings.Contains(def, "<string>io.cadence.cadenced</string>") {
		t.Errorf("expected bundle id in plist definition, got: %s", def)
	}
	if !strings.Contains(def, "<string>/usr/local/bin/cadenced</string>") {
		t.Errorf("expected binary path in plist definition, got: %s", def)
	}
	if !strings.Contains(def, "<key>KeepAlive</key>") || !strings.Contains(def, "<true/>") {
		t.Errorf("expected KeepAlive in launchd definition")
	}
}

func TestLinuxSystemdService_Definition(t *testing.T) {
	l := NewLinuxSystemdService()
	def := l.Definition("/usr/bin/cadenced")

	if !strings.Contains(def, "Description=Cadence AI Voice Dictation Daemon") {
		t.Errorf("expected Description in systemd definition, got: %s", def)
	}
	if !strings.Contains(def, "ExecStart=/usr/bin/cadenced") {
		t.Errorf("expected ExecStart in systemd definition, got: %s", def)
	}
	if !strings.Contains(def, "Restart=always") {
		t.Errorf("expected Restart=always in systemd definition, got: %s", def)
	}
}

func TestNewServiceManager(t *testing.T) {
	mgr := NewServiceManager()
	if mgr == nil {
		t.Fatal("expected non-nil ServiceManager")
	}
	path := mgr.ServiceFilePath()
	if path == "" {
		t.Error("expected non-empty ServiceFilePath")
	}
}
