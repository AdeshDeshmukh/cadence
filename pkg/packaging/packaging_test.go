package packaging

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPackaging_Specifications(t *testing.T) {
	// Root of repo is ../..
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	// 1. Homebrew Formula
	formulaPath := filepath.Join(repoRoot, "Formula", "cadence.rb")
	if err := ValidateHomebrewFormula(formulaPath); err != nil {
		t.Errorf("ValidateHomebrewFormula failed: %v", err)
	}

	// 2. Deb Control
	debControlPath := filepath.Join(repoRoot, "packaging", "deb", "DEBIAN", "control")
	if err := ValidateDebControl(debControlPath); err != nil {
		t.Errorf("ValidateDebControl failed: %v", err)
	}

	// 3. RPM Spec
	rpmSpecPath := filepath.Join(repoRoot, "packaging", "rpm", "cadence.spec")
	if err := ValidateRPMSpec(rpmSpecPath); err != nil {
		t.Errorf("ValidateRPMSpec failed: %v", err)
	}

	// 4. Windows Inno Setup
	winIssPath := filepath.Join(repoRoot, "packaging", "windows", "cadence.iss")
	if err := ValidateWindowsInstaller(winIssPath); err != nil {
		t.Errorf("ValidateWindowsInstaller failed: %v", err)
	}
}

func TestPackaging_BuildDebArchive(t *testing.T) {
	tmpDir := t.TempDir()

	cadenceBin := filepath.Join(tmpDir, "cadence")
	cadencedBin := filepath.Join(tmpDir, "cadenced")
	serviceUnit := filepath.Join(tmpDir, "cadenced.service")

	if err := os.WriteFile(cadenceBin, []byte("#!/bin/sh\necho cadence\n"), 0755); err != nil {
		t.Fatalf("write cadenceBin: %v", err)
	}
	if err := os.WriteFile(cadencedBin, []byte("#!/bin/sh\necho cadenced\n"), 0755); err != nil {
		t.Fatalf("write cadencedBin: %v", err)
	}
	if err := os.WriteFile(serviceUnit, []byte("[Unit]\nDescription=Cadence\n"), 0644); err != nil {
		t.Fatalf("write serviceUnit: %v", err)
	}

	spec := DebSpec{
		Package:      "cadence",
		Version:      "1.0.0",
		Architecture: "amd64",
		Maintainer:   "Adesh Deshmukh <adeshkd123@gmail.com>",
		Description:  "Local AI-first voice dictation CLI and developer engine",
	}

	outDeb := filepath.Join(tmpDir, "cadence_1.0.0_amd64.deb")
	if err := BuildDebArchive(spec, cadenceBin, cadencedBin, serviceUnit, outDeb); err != nil {
		t.Fatalf("BuildDebArchive failed: %v", err)
	}

	debData, err := os.ReadFile(outDeb)
	if err != nil {
		t.Fatalf("read deb file: %v", err)
	}

	// Must start with ar signature "!<arch>\n"
	if !bytes.HasPrefix(debData, []byte("!<arch>\n")) {
		t.Fatalf("expected .deb file to start with !<arch>\\n, got: %q", string(debData[:8]))
	}

	// Must contain debian-binary, control.tar.gz, data.tar.gz
	for _, member := range []string{"debian-binary", "control.tar.gz", "data.tar.gz"} {
		if !bytes.Contains(debData, []byte(member)) {
			t.Errorf("expected .deb to contain member %q", member)
		}
	}
}
