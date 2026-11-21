package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMachO_InspectAndVerify(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Test invalid / non-Mach-O file
	dummyTxt := filepath.Join(tmpDir, "dummy.txt")
	if err := os.WriteFile(dummyTxt, []byte("plain text not a binary"), 0644); err != nil {
		t.Fatalf("failed to write dummy text: %v", err)
	}
	if _, err := InspectMachO(dummyTxt); err == nil {
		t.Fatalf("expected error inspecting plain text, got nil")
	}
	if err := VerifyUniversal(dummyTxt); err == nil {
		t.Fatalf("expected error verifying plain text, got nil")
	}

	// 2. Build minimal arm64 and amd64 darwin binaries
	srcFile := filepath.Join(tmpDir, "main.go")
	srcCode := []byte("package main\n\nfunc main() {}\n")
	if err := os.WriteFile(srcFile, srcCode, 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	arm64Bin := filepath.Join(tmpDir, "test-arm64")
	amd64Bin := filepath.Join(tmpDir, "test-amd64")

	cmdArm := exec.Command("go", "build", "-o", arm64Bin, srcFile)
	cmdArm.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64")
	if out, err := cmdArm.CombinedOutput(); err != nil {
		t.Skipf("cross-compilation to darwin/arm64 unavailable in this environment: %v (%s)", err, string(out))
		return
	}

	cmdAmd := exec.Command("go", "build", "-o", amd64Bin, srcFile)
	cmdAmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=amd64")
	if out, err := cmdAmd.CombinedOutput(); err != nil {
		t.Skipf("cross-compilation to darwin/amd64 unavailable in this environment: %v (%s)", err, string(out))
		return
	}

	// 3. Inspect single-architecture Mach-O binaries
	infoArm, err := InspectMachO(arm64Bin)
	if err != nil {
		t.Fatalf("inspect arm64 binary failed: %v", err)
	}
	if infoArm.IsFat {
		t.Fatalf("expected single-arch binary, got isFat=true")
	}
	if !infoArm.HasArm64 || infoArm.HasAmd64 {
		t.Fatalf("expected only arm64 architecture, got: %v", infoArm.Architectures)
	}

	// Verifying single binary as universal should fail
	if err := VerifyUniversal(arm64Bin); err == nil {
		t.Fatalf("expected VerifyUniversal to fail on single-arch binary")
	}

	infoAmd, err := InspectMachO(amd64Bin)
	if err != nil {
		t.Fatalf("inspect amd64 binary failed: %v", err)
	}
	if infoAmd.IsFat {
		t.Fatalf("expected single-arch binary, got isFat=true")
	}
	if !infoAmd.HasAmd64 || infoAmd.HasArm64 {
		t.Fatalf("expected only x86_64 architecture, got: %v", infoAmd.Architectures)
	}

	// 4. Test pure-Go PackFatMachO
	fatPureGo := filepath.Join(tmpDir, "universal-purego")
	if err := PackFatMachO(arm64Bin, amd64Bin, fatPureGo); err != nil {
		t.Fatalf("PackFatMachO failed: %v", err)
	}

	infoPureGo, err := InspectMachO(fatPureGo)
	if err != nil {
		t.Fatalf("InspectMachO on pure-Go fat binary failed: %v", err)
	}
	if !infoPureGo.IsFat {
		t.Fatalf("expected isFat=true for PackFatMachO output")
	}
	if !infoPureGo.HasArm64 || !infoPureGo.HasAmd64 {
		t.Fatalf("expected both arm64 and x86_64, got: %v", infoPureGo.Architectures)
	}
	if err := VerifyUniversal(fatPureGo); err != nil {
		t.Fatalf("VerifyUniversal failed on pure-Go fat binary: %v", err)
	}

	// 5. Test CreateUniversal (prefers lipo if present, else fallback)
	fatUniversal := filepath.Join(tmpDir, "universal-binary")
	if err := CreateUniversal(arm64Bin, amd64Bin, fatUniversal); err != nil {
		t.Fatalf("CreateUniversal failed: %v", err)
	}
	if err := VerifyUniversal(fatUniversal); err != nil {
		t.Fatalf("VerifyUniversal failed on CreateUniversal output: %v", err)
	}

	// 6. If running natively on macOS with lipo, verify with system lipo tool
	if runtime.GOOS == "darwin" {
		if lipoPath, err := exec.LookPath("lipo"); err == nil && lipoPath != "" {
			cmd := exec.Command(lipoPath, "-info", fatUniversal)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("lipo -info failed on universal binary: %v (%s)", err, string(out))
			}
			outStr := string(out)
			if !strings.Contains(outStr, "arm64") || !strings.Contains(outStr, "x86_64") {
				t.Errorf("expected lipo -info to contain both arm64 and x86_64, got: %s", outStr)
			}
		}
	}
}

func TestBuildDarwinUniversal_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	outBin := filepath.Join(tmpDir, "nonexistent-out")

	// Building nonexistent package should fail
	if err := BuildDarwinUniversal("./nonexistent/package/path", outBin); err == nil {
		t.Fatalf("expected error building invalid package path, got nil")
	}
}
