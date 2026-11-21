package build

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// FatMagic is the Mach-O fat universal binary magic header constant (0xcafebabe).
	FatMagic uint32 = 0xcafebabe

	// AlignArm64Power is the standard 16KB (2^14) page alignment exponent for Apple Silicon.
	AlignArm64Power uint32 = 14
	// AlignAmd64Power is the standard 4KB or 8KB (2^13) alignment exponent for x86_64.
	AlignAmd64Power uint32 = 13
)

// UniversalBinaryInfo summarizes architecture details of a Mach-O executable.
type UniversalBinaryInfo struct {
	Path          string   `json:"path"`
	Size          int64    `json:"size"`
	IsFat         bool     `json:"is_fat"`
	Architectures []string `json:"architectures"`
	HasArm64      bool     `json:"has_arm64"`
	HasAmd64      bool     `json:"has_amd64"`
}

// InspectMachO analyzes a binary file to determine whether it is a Mach-O single or universal binary.
func InspectMachO(path string) (*UniversalBinaryInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat binary: %w", err)
	}

	info := &UniversalBinaryInfo{
		Path: path,
		Size: fi.Size(),
	}

	// Try reading as a universal FAT binary first
	fat, err := macho.OpenFat(path)
	if err == nil {
		defer fat.Close()
		info.IsFat = true
		for _, arch := range fat.Arches {
			switch arch.Cpu {
			case macho.CpuArm64:
				info.HasArm64 = true
				info.Architectures = append(info.Architectures, "arm64")
			case macho.CpuAmd64:
				info.HasAmd64 = true
				info.Architectures = append(info.Architectures, "x86_64")
			default:
				info.Architectures = append(info.Architectures, arch.Cpu.String())
			}
		}
		return info, nil
	}

	// If not fat, try reading as a single-architecture Mach-O file
	single, err := macho.Open(path)
	if err == nil {
		defer single.Close()
		info.IsFat = false
		switch single.Cpu {
		case macho.CpuArm64:
			info.HasArm64 = true
			info.Architectures = append(info.Architectures, "arm64")
		case macho.CpuAmd64:
			info.HasAmd64 = true
			info.Architectures = append(info.Architectures, "x86_64")
		default:
			info.Architectures = append(info.Architectures, single.Cpu.String())
		}
		return info, nil
	}

	return nil, fmt.Errorf("%s is not a recognized Mach-O binary", path)
}

// VerifyUniversal checks that the specified binary is a universal Mach-O containing both arm64 and x86_64.
func VerifyUniversal(path string) error {
	info, err := InspectMachO(path)
	if err != nil {
		return err
	}
	if !info.IsFat {
		return fmt.Errorf("%s is a single-architecture binary (%s), not universal", path, strings.Join(info.Architectures, ", "))
	}
	if !info.HasArm64 || !info.HasAmd64 {
		return fmt.Errorf("%s is missing required architectures (arm64: %t, x86_64: %t)", path, info.HasArm64, info.HasAmd64)
	}
	return nil
}

// CreateUniversal merges arm64 and amd64 binaries into a single universal binary.
// It prefers the host lipo utility when available, and falls back to pure-Go Mach-O packing.
func CreateUniversal(arm64Path, amd64Path, outPath string) error {
	// 1. Try host lipo if installed
	if lipoPath, err := exec.LookPath("lipo"); err == nil && lipoPath != "" {
		cmd := exec.Command(lipoPath, "-create", "-output", outPath, arm64Path, amd64Path)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			return VerifyUniversal(outPath)
		}
	}

	// 2. Fall back to pure-Go Mach-O FAT packager
	if err := PackFatMachO(arm64Path, amd64Path, outPath); err != nil {
		return fmt.Errorf("pack universal binary: %w", err)
	}

	return VerifyUniversal(outPath)
}

// Mach-O Fat header structs
type fatHeader struct {
	Magic uint32
	NArch uint32
}

type fatArchEntry struct {
	Cpu    uint32
	SubCpu uint32
	Offset uint32
	Size   uint32
	Align  uint32
}

// PackFatMachO constructs a valid universal Mach-O binary from arm64 and amd64 input files in pure Go.
func PackFatMachO(arm64Path, amd64Path, outPath string) error {
	arm64Data, err := os.ReadFile(arm64Path)
	if err != nil {
		return fmt.Errorf("read arm64 binary: %w", err)
	}
	amd64Data, err := os.ReadFile(amd64Path)
	if err != nil {
		return fmt.Errorf("read amd64 binary: %w", err)
	}

	// Read CPU/SubCPU from Mach-O files
	mArm64, err := macho.Open(arm64Path)
	if err != nil {
		return fmt.Errorf("open arm64 Mach-O: %w", err)
	}
	defer mArm64.Close()

	mAmd64, err := macho.Open(amd64Path)
	if err != nil {
		return fmt.Errorf("open amd64 Mach-O: %w", err)
	}
	defer mAmd64.Close()

	if mArm64.Cpu != macho.CpuArm64 {
		return fmt.Errorf("expected arm64 binary in %s, got %v", arm64Path, mArm64.Cpu)
	}
	if mAmd64.Cpu != macho.CpuAmd64 {
		return fmt.Errorf("expected amd64 binary in %s, got %v", amd64Path, mAmd64.Cpu)
	}

	// Layout architectures:
	// Apple convention: x86_64 first (aligned to 8192 or 16384), followed by arm64 (aligned to 16384).
	const pageSize = uint32(16384)
	offset1 := pageSize
	size1 := uint32(len(amd64Data))

	offset2 := ((offset1 + size1 + pageSize - 1) / pageSize) * pageSize
	size2 := uint32(len(arm64Data))

	header := fatHeader{
		Magic: FatMagic,
		NArch: 2,
	}

	entries := []fatArchEntry{
		{
			Cpu:    uint32(mAmd64.Cpu),
			SubCpu: mAmd64.SubCpu,
			Offset: offset1,
			Size:   size1,
			Align:  AlignAmd64Power,
		},
		{
			Cpu:    uint32(mArm64.Cpu),
			SubCpu: mArm64.SubCpu,
			Offset: offset2,
			Size:   size2,
			Align:  AlignArm64Power,
		},
	}

	totalSize := offset2 + size2
	buf := make([]byte, totalSize)

	// Write FAT header
	var hdrBuf bytes.Buffer
	if err := binary.Write(&hdrBuf, binary.BigEndian, header); err != nil {
		return err
	}
	for _, entry := range entries {
		if err := binary.Write(&hdrBuf, binary.BigEndian, entry); err != nil {
			return err
		}
	}
	copy(buf[:hdrBuf.Len()], hdrBuf.Bytes())

	// Copy payloads
	copy(buf[offset1:offset1+size1], amd64Data)
	copy(buf[offset2:offset2+size2], arm64Data)

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(outPath, buf, 0755)
}

// BuildDarwinUniversal compiles the Go package for darwin/arm64 and darwin/amd64,
// and packages them into a universal binary at outPath.
func BuildDarwinUniversal(pkgPath, outPath string) error {
	tmpDir, err := os.MkdirTemp("", "cadence-universal-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	arm64Bin := filepath.Join(tmpDir, "bin-arm64")
	amd64Bin := filepath.Join(tmpDir, "bin-amd64")

	// Build arm64
	cmdArm := exec.Command("go", "build", "-o", arm64Bin, pkgPath)
	cmdArm.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64")
	if out, err := cmdArm.CombinedOutput(); err != nil {
		return fmt.Errorf("build darwin/arm64: %s: %w", string(out), err)
	}

	// Build amd64
	cmdAmd := exec.Command("go", "build", "-o", amd64Bin, pkgPath)
	cmdAmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=amd64")
	if out, err := cmdAmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build darwin/amd64: %s: %w", string(out), err)
	}

	return CreateUniversal(arm64Bin, amd64Bin, outPath)
}

// CopyFile is a helper to copy file contents preserving permissions.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// EnsureErrors exported for test coverage
var ErrNotMachO = errors.New("file is not a Mach-O binary")
