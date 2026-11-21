package packaging

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// DebSpec defines the metadata and components required to build a Debian package.
type DebSpec struct {
	Package      string
	Version      string
	Architecture string
	Maintainer   string
	Description  string
	ControlExtra string
}

// BuildDebArchive builds a pure-Go standard .deb package from binaries and control scripts.
func BuildDebArchive(spec DebSpec, cadenceBin, cadencedBin, serviceUnit, outPath string) error {
	// 1. Build debian-binary payload
	debianBinary := []byte("2.0\n")

	// 2. Build control.tar.gz
	controlTarGz, err := createControlTarGz(spec)
	if err != nil {
		return fmt.Errorf("create control.tar.gz: %w", err)
	}

	// 3. Build data.tar.gz
	dataTarGz, err := createDataTarGz(cadenceBin, cadencedBin, serviceUnit)
	if err != nil {
		return fmt.Errorf("create data.tar.gz: %w", err)
	}

	// 4. Assemble ar archive
	var arBuf bytes.Buffer
	arBuf.WriteString("!<arch>\n")

	if err := writeArMember(&arBuf, "debian-binary", debianBinary, 0644); err != nil {
		return err
	}
	if err := writeArMember(&arBuf, "control.tar.gz", controlTarGz, 0644); err != nil {
		return err
	}
	if err := writeArMember(&arBuf, "data.tar.gz", dataTarGz, 0644); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(outPath, arBuf.Bytes(), 0644)
}

func createControlTarGz(spec DebSpec) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	controlContent := fmt.Sprintf("Package: %s\nVersion: %s\nSection: utils\nPriority: optional\nArchitecture: %s\nMaintainer: %s\nDescription: %s\n%s",
		spec.Package, spec.Version, spec.Architecture, spec.Maintainer, spec.Description, spec.ControlExtra)

	hdr := &tar.Header{
		Name:     "./control",
		Mode:     0644,
		Size:     int64(len(controlContent)),
		ModTime:  time.Now().UTC(),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(controlContent)); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func createDataTarGz(cadenceBin, cadencedBin, serviceUnit string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	now := time.Now().UTC()

	// Add directory structures
	dirs := []string{"./usr", "./usr/bin", "./usr/lib", "./usr/lib/systemd", "./usr/lib/systemd/user"}
	for _, dir := range dirs {
		hdr := &tar.Header{
			Name:     dir,
			Mode:     0755,
			Typeflag: tar.TypeDir,
			ModTime:  now,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
	}

	// Helper to add executable file
	addFile := func(targetPath, srcPath string, mode int64) error {
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", srcPath, err)
		}
		hdr := &tar.Header{
			Name:     targetPath,
			Mode:     mode,
			Size:     int64(len(data)),
			ModTime:  now,
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	}

	if err := addFile("./usr/bin/cadence", cadenceBin, 0755); err != nil {
		return nil, err
	}
	if err := addFile("./usr/bin/cadenced", cadencedBin, 0755); err != nil {
		return nil, err
	}
	if serviceUnit != "" {
		if err := addFile("./usr/lib/systemd/user/cadenced.service", serviceUnit, 0644); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeArMember writes an ar format archive entry (60 bytes header + payload + padding).
func writeArMember(w io.Writer, name string, data []byte, mode int) error {
	mtime := time.Now().Unix()
	hdr := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n",
		name,
		mtime,
		0,
		0,
		mode,
		len(data),
	)
	if _, err := io.WriteString(w, hdr); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if len(data)%2 != 0 {
		if _, err := w.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return nil
}
