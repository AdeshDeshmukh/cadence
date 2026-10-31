package storage

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// DetectFormat infers format from filename extension or content inspection.
func DetectFormat(filename string, content []byte) string {
	if filename != "" {
		ext := strings.ToLower(filepath.Ext(filename))
		switch ext {
		case ".csv":
			return "csv"
		case ".json":
			return "json"
		case ".txt", ".tsv", ".conf", ".text":
			return "txt"
		}
	}

	trimmed := bytes.TrimSpace(content)
	if len(trimmed) > 0 {
		if trimmed[0] == '{' || trimmed[0] == '[' {
			return "json"
		}
		firstLine := string(trimmed)
		if idx := strings.IndexByte(firstLine, '\n'); idx != -1 {
			firstLine = firstLine[:idx]
		}
		if strings.Contains(firstLine, ",") {
			return "csv"
		}
	}

	return "txt"
}

// ParseVocabulary parses vocabulary entries from data in CSV, JSON, or text format.
func ParseVocabulary(r io.Reader, format string) ([]VocabularyEntry, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read vocabulary data: %w", err)
	}

	normFormat := strings.ToLower(strings.TrimSpace(format))
	if normFormat == "" || normFormat == "auto" {
		normFormat = DetectFormat("", data)
	}

	switch normFormat {
	case "csv":
		return parseCSV(data)
	case "json":
		return parseJSON(data)
	case "txt", "text", "plain":
		return parsePlainText(data)
	default:
		return nil, fmt.Errorf("unsupported vocabulary format: %s (supported: csv, json, txt)", format)
	}
}

func parseCSV(data []byte) ([]VocabularyEntry, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}

	var out []VocabularyEntry
	for i, row := range records {
		if len(row) < 2 {
			continue
		}
		spoken := strings.TrimSpace(row[0])
		canonical := strings.TrimSpace(row[1])
		if spoken == "" || canonical == "" {
			continue
		}
		// Skip header line if present
		if i == 0 && (strings.EqualFold(spoken, "spoken") || strings.EqualFold(spoken, "term")) {
			continue
		}
		out = append(out, VocabularyEntry{Spoken: spoken, Canonical: canonical})
	}
	return out, nil
}

func parseJSON(data []byte) ([]VocabularyEntry, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}

	// Try array of objects
	if trimmed[0] == '[' {
		var list []VocabularyEntry
		if err := json.Unmarshal(trimmed, &list); err == nil {
			var out []VocabularyEntry
			for _, e := range list {
				spoken := strings.TrimSpace(e.Spoken)
				canonical := strings.TrimSpace(e.Canonical)
				if spoken != "" && canonical != "" {
					out = append(out, VocabularyEntry{Spoken: spoken, Canonical: canonical})
				}
			}
			return out, nil
		}
	}

	// Try map[string]string
	var kv map[string]string
	if err := json.Unmarshal(trimmed, &kv); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	var out []VocabularyEntry
	for spoken, canonical := range kv {
		sp := strings.TrimSpace(spoken)
		ca := strings.TrimSpace(canonical)
		if sp != "" && ca != "" {
			out = append(out, VocabularyEntry{Spoken: sp, Canonical: ca})
		}
	}
	return out, nil
}

func parsePlainText(data []byte) ([]VocabularyEntry, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var out []VocabularyEntry

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, ";") {
			continue
		}

		var spoken, canonical string
		if strings.Contains(line, "->") {
			parts := strings.SplitN(line, "->", 2)
			spoken, canonical = parts[0], parts[1]
		} else if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			spoken, canonical = parts[0], parts[1]
		} else if strings.Contains(line, "\t") {
			parts := strings.SplitN(line, "\t", 2)
			spoken, canonical = parts[0], parts[1]
		} else if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			spoken, canonical = parts[0], parts[1]
		} else {
			continue
		}

		spoken = strings.TrimSpace(spoken)
		canonical = strings.TrimSpace(canonical)
		if spoken != "" && canonical != "" {
			out = append(out, VocabularyEntry{Spoken: spoken, Canonical: canonical})
		}
	}

	return out, scanner.Err()
}

// FormatVocabulary serializes vocabulary entries into the specified format.
func FormatVocabulary(entries []VocabularyEntry, format string, w io.Writer) error {
	normFormat := strings.ToLower(strings.TrimSpace(format))
	if normFormat == "" || normFormat == "auto" {
		normFormat = "json"
	}

	switch normFormat {
	case "csv":
		cw := csv.NewWriter(w)
		if err := cw.Write([]string{"spoken", "canonical"}); err != nil {
			return err
		}
		for _, e := range entries {
			if err := cw.Write([]string{e.Spoken, e.Canonical}); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()

	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)

	case "txt", "text", "plain":
		bw := bufio.NewWriter(w)
		for _, e := range entries {
			if _, err := fmt.Fprintf(bw, "%s = %s\n", e.Spoken, e.Canonical); err != nil {
				return err
			}
		}
		return bw.Flush()

	default:
		return fmt.Errorf("unsupported export format: %s (supported: csv, json, txt)", format)
	}
}

// ImportVocabulary imports vocabulary entries from a reader into the store.
func ImportVocabulary(store Store, r io.Reader, format string) (int, error) {
	entries, err := ParseVocabulary(r, format)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, e := range entries {
		if strings.TrimSpace(e.Spoken) == "" || strings.TrimSpace(e.Canonical) == "" {
			continue
		}
		if err := store.SetVocabulary(e); err != nil {
			return count, fmt.Errorf("set vocabulary %q -> %q: %w", e.Spoken, e.Canonical, err)
		}
		count++
	}
	return count, nil
}

// ExportVocabulary exports all vocabulary entries from the store to the writer.
func ExportVocabulary(store Store, format string, w io.Writer) error {
	entries, err := store.Vocabulary()
	if err != nil {
		return fmt.Errorf("retrieve vocabulary: %w", err)
	}
	return FormatVocabulary(entries, format, w)
}
