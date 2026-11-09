package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/system"
)

func TestPermissionEngine_DefaultRules(t *testing.T) {
	pe := NewDefaultPermissionEngine()
	rules := pe.Rules()
	if len(rules) < 6 {
		t.Fatalf("expected at least 6 default rules, got %d", len(rules))
	}

	// 1. fs read should be allowed
	d := pe.Check("fs", "read", map[string]any{"path": "sample.txt"})
	if !d.Granted || d.Level != PermissionAllow {
		t.Fatalf("expected fs.read to be allowed, got %+v", d)
	}

	// 2. clipboard set should be allowed
	d = pe.Check("clipboard", "set", map[string]any{"text": "cadence"})
	if !d.Granted || d.Level != PermissionAllow {
		t.Fatalf("expected clipboard.set to be allowed, got %+v", d)
	}

	// 3. exec should require user confirmation (ask)
	d = pe.Check("exec", "run", map[string]any{"command": "echo test"})
	if d.Granted || d.Level != PermissionAsk || !d.RequiresUser {
		t.Fatalf("expected exec to require confirmation, got %+v", d)
	}
}

func TestPermissionEngine_GrantAndRevoke(t *testing.T) {
	pe := NewDefaultPermissionEngine()

	// 1. Explicitly deny fs delete
	err := pe.Grant(PermissionRule{
		Tool:   "fs",
		Action: "delete",
		Level:  PermissionDeny,
	})
	if err != nil {
		t.Fatalf("grant failed: %v", err)
	}

	d := pe.Check("fs", "delete", map[string]any{"path": "file.txt"})
	if d.Granted || d.Level != PermissionDeny {
		t.Fatalf("expected fs.delete to be denied, got %+v", d)
	}

	// 2. Revoke explicit rule
	revoked := pe.Revoke("fs", "delete")
	if !revoked {
		t.Fatalf("expected rule to be revoked")
	}

	// 3. Fallback to generic fs wildcard (which is allow in defaults)
	d = pe.Check("fs", "delete", map[string]any{"path": "file.txt"})
	if !d.Granted {
		t.Fatalf("expected fs to fallback to generic allow rule, got %+v", d)
	}

	// 4. Revoke non-existent rule
	if pe.Revoke("unknown", "action") {
		t.Fatalf("expected revoke of unknown rule to return false")
	}
}

func TestPermissionEngine_SpecificPrecedence(t *testing.T) {
	pe := NewPermissionEngine(PermissionDeny)

	// Generic wildcard rule: allow all
	_ = pe.Grant(PermissionRule{Tool: "*", Action: "*", Level: PermissionAllow})
	// Specific tool rule: ask for clipboard
	_ = pe.Grant(PermissionRule{Tool: "clipboard", Action: "*", Level: PermissionAsk})
	// Exact tool + action: deny clipboard clear
	_ = pe.Grant(PermissionRule{Tool: "clipboard", Action: "clear", Level: PermissionDeny})

	// 1. Random tool inherits wildcard allow
	d := pe.Check("random_tool", "any", nil)
	if !d.Granted || d.Level != PermissionAllow {
		t.Fatalf("expected random tool to match wildcard allow, got %+v", d)
	}

	// 2. Clipboard get matches clipboard.* (ask)
	d = pe.Check("clipboard", "get", nil)
	if d.Granted || d.Level != PermissionAsk {
		t.Fatalf("expected clipboard get to match tool wildcard ask, got %+v", d)
	}

	// 3. Clipboard clear matches exact rule (deny)
	d = pe.Check("clipboard", "clear", nil)
	if d.Granted || d.Level != PermissionDeny {
		t.Fatalf("expected clipboard clear to match exact rule deny, got %+v", d)
	}
}

func TestPermissionEngine_SensitivePaths(t *testing.T) {
	pe := NewDefaultPermissionEngine()

	// Normal path is allowed
	d := pe.Check("fs", "read", map[string]any{"path": "/home/user/project/main.go"})
	if !d.Granted {
		t.Fatalf("expected project path to be allowed, got %+v", d)
	}

	// Sensitive paths must be blocked
	sensitive := []string{
		"/etc/shadow",
		"/etc/sudoers",
		"/Users/user/.ssh/id_rsa",
		"/Users/user/.ssh/id_ed25519",
		"/Users/user/.aws/credentials",
		"/Users/user/.kube/config",
	}

	for _, p := range sensitive {
		d = pe.Check("fs", "read", map[string]any{"path": p})
		if d.Granted || d.Level != PermissionDeny {
			t.Fatalf("expected path %q to be denied by sensitive guard, got %+v", p, d)
		}
	}
}

func TestPermissionEngine_PathScoping(t *testing.T) {
	pe := NewPermissionEngine(PermissionDeny)

	allowedDir := "/workspace/allowed"
	_ = pe.Grant(PermissionRule{
		Tool:   "fs",
		Action: "*",
		Level:  PermissionAllow,
		Scope:  []string{allowedDir},
	})

	// Inside scope: allowed
	inside := filepath.Join(allowedDir, "src", "main.go")
	d := pe.Check("fs", "read", map[string]any{"path": inside})
	if !d.Granted {
		t.Fatalf("expected inside scope path to be granted, got %+v", d)
	}

	// Outside scope: denied
	outside := "/workspace/forbidden/secrets.env"
	d = pe.Check("fs", "read", map[string]any{"path": outside})
	if d.Granted || d.Level != PermissionDeny {
		t.Fatalf("expected outside scope path to be denied, got %+v", d)
	}
}

func TestPermissionEngine_CommandScoping(t *testing.T) {
	pe := NewPermissionEngine(PermissionDeny)

	_ = pe.Grant(PermissionRule{
		Tool:   "exec",
		Action: "*",
		Level:  PermissionAllow,
		Scope:  []string{"git", "go"},
	})

	// Allowed command prefix
	d := pe.Check("exec", "run", map[string]any{"command": "git status"})
	if !d.Granted {
		t.Fatalf("expected git status to be allowed, got %+v", d)
	}

	d = pe.Check("exec", "run", map[string]any{"command": "go test ./..."})
	if !d.Granted {
		t.Fatalf("expected go test to be allowed, got %+v", d)
	}

	// Forbidden command
	d = pe.Check("exec", "run", map[string]any{"command": "rm -rf /"})
	if d.Granted || d.Level != PermissionDeny {
		t.Fatalf("expected rm command to be denied by command scope, got %+v", d)
	}
}

func TestPermissionEngine_UserApproval(t *testing.T) {
	pe := NewDefaultPermissionEngine()

	// exec without approval requires confirmation
	d := pe.Check("exec", "run", map[string]any{"command": "npm install"})
	if d.Granted || d.Level != PermissionAsk {
		t.Fatalf("expected unapproved exec to require confirmation, got %+v", d)
	}

	// exec with approved=true succeeds
	d = pe.Check("exec", "run", map[string]any{"command": "npm install", "approved": true})
	if !d.Granted || d.Level != PermissionAsk {
		t.Fatalf("expected approved exec to be granted, got %+v", d)
	}
}

func TestRegistry_PermissionEnforcement(t *testing.T) {
	mockCtrl := system.NewMockController()
	reg := NewRegistry(mockCtrl, nil)
	ctx := context.Background()

	// 1. Explicitly deny clipboard
	_ = reg.Permissions().Grant(PermissionRule{
		Tool:   "clipboard",
		Action: "*",
		Level:  PermissionDeny,
	})

	res, err := reg.Call(ctx, "clipboard", map[string]any{"action": "set", "text": "blocked"})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected call to be denied by permission engine")
	}

	// 2. Re-grant allow
	_ = reg.Permissions().Grant(PermissionRule{
		Tool:   "clipboard",
		Action: "*",
		Level:  PermissionAllow,
	})

	res, err = reg.Call(ctx, "clipboard", map[string]any{"action": "set", "text": "allowed"})
	if err != nil || !res.Success {
		t.Fatalf("expected re-granted call to succeed, got res: %+v", res)
	}

	// 3. Sensitive path enforcement via Registry Call
	tmpDir := t.TempDir()
	fakeShadow := filepath.Join(tmpDir, "shadow")
	_ = os.WriteFile(fakeShadow, []byte("root:*:12345:0:99999:7:::"), 0600)

	res, err = reg.Call(ctx, "fs", map[string]any{
		"action": "read",
		"path":   fakeShadow,
	})
	// Normal temp file named shadow should be allowed unless in /etc/shadow
	if err != nil || !res.Success {
		t.Fatalf("normal temp shadow should be allowed: %+v", res)
	}

	// Absolute /etc/shadow should be denied
	res, err = reg.Call(ctx, "fs", map[string]any{
		"action": "read",
		"path":   "/etc/shadow",
	})
	if res.Success {
		t.Fatalf("expected /etc/shadow access to be denied by permission policy")
	}
}
