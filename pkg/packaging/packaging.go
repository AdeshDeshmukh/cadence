package packaging

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// PackageMetadata contains common release metadata across platform package formats.
type PackageMetadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	Maintainer  string `json:"maintainer"`
	License     string `json:"license"`
}

// DefaultMetadata returns the canonical metadata for Cadence v1.0.0.
func DefaultMetadata() PackageMetadata {
	return PackageMetadata{
		Name:        "cadence",
		Version:     "1.0.0",
		Description: "Local AI-first voice dictation CLI and developer engine",
		Homepage:    "https://github.com/AdeshDeshmukh/cadence",
		Maintainer:  "Adesh Deshmukh <adeshkd123@gmail.com>",
		License:     "Apache-2.0",
	}
}

// ValidateHomebrewFormula verifies required fields and structure of a Ruby Homebrew formula.
func ValidateHomebrewFormula(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read formula: %w", err)
	}
	content := string(data)

	required := []string{
		"class Cadence < Formula",
		"desc ",
		"homepage ",
		"url ",
		"sha256 ",
		"license ",
		"def install",
		"service do",
		"opt_bin/\"cadenced\"",
		"test do",
	}

	for _, req := range required {
		if !strings.Contains(content, req) {
			return fmt.Errorf("formula missing required element: %q", req)
		}
	}
	return nil
}

// ValidateDebControl verifies the fields in a Debian control file.
func ValidateDebControl(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read deb control: %w", err)
	}
	content := string(data)

	required := []string{
		"Package: cadence",
		"Version: ",
		"Architecture: ",
		"Maintainer: ",
		"Description: ",
	}

	for _, req := range required {
		if !strings.Contains(content, req) {
			return fmt.Errorf("deb control missing required field: %q", req)
		}
	}
	return nil
}

// ValidateRPMSpec verifies required sections and macros in an RPM spec file.
func ValidateRPMSpec(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read rpm spec: %w", err)
	}
	content := string(data)

	required := []string{
		"Name:",
		"Version:",
		"Summary:",
		"License:",
		"URL:",
		"%description",
		"%prep",
		"%build",
		"%install",
		"%files",
		"%post",
		"%preun",
		"%changelog",
	}

	for _, req := range required {
		if !strings.Contains(content, req) {
			return fmt.Errorf("rpm spec missing required section: %q", req)
		}
	}
	return nil
}

// ValidateWindowsInstaller verifies required sections in the Inno Setup .iss definition.
func ValidateWindowsInstaller(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read inno setup file: %w", err)
	}
	content := string(data)

	required := []string{
		"[Setup]",
		"AppName=",
		"AppVersion=",
		"OutputBaseFilename=",
		"[Files]",
		"cadence.exe",
		"cadenced.exe",
		"[Registry]",
		"[Icons]",
	}

	for _, req := range required {
		if !strings.Contains(content, req) {
			return fmt.Errorf("windows installer spec missing required section: %q", req)
		}
	}
	return nil
}

// ErrMissingSpec indicates a required package spec was not found.
var ErrMissingSpec = errors.New("missing packaging specification")
