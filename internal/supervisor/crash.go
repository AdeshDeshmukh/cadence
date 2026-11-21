package supervisor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// CrashReport encapsulates diagnostic telemetry captured during an abnormal process termination.
type CrashReport struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Reason         string    `json:"reason"`
	Error          string    `json:"error"`
	StackTrace     string    `json:"stack_trace"`
	NumGoroutines  int       `json:"num_goroutines"`
	AllocBytes     uint64    `json:"alloc_bytes"`
	TotalAlloc     uint64    `json:"total_alloc_bytes"`
	SysBytes       uint64    `json:"sys_bytes"`
	RestartAttempt int       `json:"restart_attempt"`
}

// DefaultCrashDir returns the standard storage directory for crash dumps.
func DefaultCrashDir() string {
	if env := os.Getenv("CADENCE_CRASH_DIR"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "cadence", "crashes")
	}
	return filepath.Join(home, ".local", "state", "cadence", "crashes")
}

// CaptureCrash creates a new CrashReport with current runtime statistics and stack trace.
func CaptureCrash(reason, errStr, stack string, attempt int) CrashReport {
	if stack == "" {
		buf := make([]byte, 16*1024)
		n := runtime.Stack(buf, false)
		stack = string(buf[:n])
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	now := time.Now().UTC()
	id := fmt.Sprintf("crash-%d", now.UnixNano())

	return CrashReport{
		ID:             id,
		Timestamp:      now,
		Reason:         reason,
		Error:          errStr,
		StackTrace:     stack,
		NumGoroutines:  runtime.NumGoroutine(),
		AllocBytes:     mem.Alloc,
		TotalAlloc:     mem.TotalAlloc,
		SysBytes:       mem.Sys,
		RestartAttempt: attempt,
	}
}

// WriteCrashDump persists a CrashReport to disk in JSON format.
func WriteCrashDump(dir string, r *CrashReport) (string, error) {
	if dir == "" {
		dir = DefaultCrashDir()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create crash dir: %w", err)
	}

	filename := fmt.Sprintf("%s.json", r.ID)
	fullPath := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal crash report: %w", err)
	}

	if err := os.WriteFile(fullPath, data, 0600); err != nil {
		return "", fmt.Errorf("write crash dump: %w", err)
	}

	return fullPath, nil
}

// ReadCrashDumps loads all saved crash dumps from dir ordered newest first.
func ReadCrashDumps(dir string) ([]CrashReport, error) {
	if dir == "" {
		dir = DefaultCrashDir()
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var reports []CrashReport
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		var r CrashReport
		if err := json.Unmarshal(data, &r); err == nil {
			reports = append(reports, r)
		}
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Timestamp.After(reports[j].Timestamp)
	})

	return reports, nil
}
