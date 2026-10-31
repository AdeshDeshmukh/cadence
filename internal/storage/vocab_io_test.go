package storage_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

func TestVocabIO_CSV(t *testing.T) {
	csvData := `spoken,canonical
cube control,kubectl
docker compose,docker-compose
postgres,PostgreSQL
`
	entries, err := storage.ParseVocabulary(strings.NewReader(csvData), "csv")
	if err != nil {
		t.Fatalf("ParseVocabulary(csv): %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Spoken != "cube control" || entries[0].Canonical != "kubectl" {
		t.Errorf("entry 0 mismatch: %+v", entries[0])
	}

	var buf bytes.Buffer
	if err := storage.FormatVocabulary(entries, "csv", &buf); err != nil {
		t.Fatalf("FormatVocabulary(csv): %v", err)
	}
	formatted := buf.String()
	if !strings.Contains(formatted, "spoken,canonical") || !strings.Contains(formatted, "docker-compose") {
		t.Fatalf("unexpected formatted csv: %s", formatted)
	}
}

func TestVocabIO_JSON(t *testing.T) {
	// 1. Array of objects
	jsonArray := `[
		{"spoken": "k eight s", "canonical": "k8s"},
		{"spoken": "g r p c", "canonical": "gRPC"}
	]`
	entries, err := storage.ParseVocabulary(strings.NewReader(jsonArray), "json")
	if err != nil {
		t.Fatalf("ParseVocabulary(json array): %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// 2. Map of string to string
	jsonMap := `{"kube ctl": "kubectl", "react j s": "React.js"}`
	entriesMap, err := storage.ParseVocabulary(strings.NewReader(jsonMap), "json")
	if err != nil {
		t.Fatalf("ParseVocabulary(json map): %v", err)
	}
	if len(entriesMap) != 2 {
		t.Fatalf("expected 2 entries from map, got %d", len(entriesMap))
	}

	var buf bytes.Buffer
	if err := storage.FormatVocabulary(entries, "json", &buf); err != nil {
		t.Fatalf("FormatVocabulary(json): %v", err)
	}
	if !strings.Contains(buf.String(), "gRPC") {
		t.Fatalf("unexpected formatted json: %s", buf.String())
	}
}

func TestVocabIO_PlainText(t *testing.T) {
	textData := `# Custom vocabulary file
// Developer abbreviations
k8s = Kubernetes
golang -> Go
docker compose : docker-compose
tab key	Tabulator

; ignored line
`
	entries, err := storage.ParseVocabulary(strings.NewReader(textData), "txt")
	if err != nil {
		t.Fatalf("ParseVocabulary(txt): %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}
	if entries[0].Spoken != "k8s" || entries[0].Canonical != "Kubernetes" {
		t.Errorf("entry 0 mismatch: %+v", entries[0])
	}
	if entries[1].Spoken != "golang" || entries[1].Canonical != "Go" {
		t.Errorf("entry 1 mismatch: %+v", entries[1])
	}

	var buf bytes.Buffer
	if err := storage.FormatVocabulary(entries, "txt", &buf); err != nil {
		t.Fatalf("FormatVocabulary(txt): %v", err)
	}
	formatted := buf.String()
	if !strings.Contains(formatted, "k8s = Kubernetes") || !strings.Contains(formatted, "golang = Go") {
		t.Fatalf("unexpected formatted text: %s", formatted)
	}
}

func TestVocabIO_DetectFormat(t *testing.T) {
	tests := []struct {
		filename string
		content  string
		expected string
	}{
		{"vocab.csv", "a,b", "csv"},
		{"vocab.json", "{}", "json"},
		{"vocab.txt", "a = b", "txt"},
		{"", `[{"spoken": "a", "canonical": "b"}]`, "json"},
		{"", `{"a": "b"}`, "json"},
		{"", "hello,world\nfoo,bar", "csv"},
		{"", "hello = world\n", "txt"},
	}

	for _, tt := range tests {
		got := storage.DetectFormat(tt.filename, []byte(tt.content))
		if got != tt.expected {
			t.Errorf("DetectFormat(%q, %q) = %q, expected %q", tt.filename, tt.content, got, tt.expected)
		}
	}
}

func TestVocabIO_ImportAndExport(t *testing.T) {
	mem := storage.NewMemory()

	csvContent := `spoken,canonical
grpc,gRPC
postgres,PostgreSQL
`
	count, err := storage.ImportVocabulary(mem, strings.NewReader(csvContent), "csv")
	if err != nil {
		t.Fatalf("ImportVocabulary failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 imported entries, got %d", count)
	}

	var exported bytes.Buffer
	if err := storage.ExportVocabulary(mem, "txt", &exported); err != nil {
		t.Fatalf("ExportVocabulary failed: %v", err)
	}

	if !strings.Contains(exported.String(), "grpc = gRPC") || !strings.Contains(exported.String(), "postgres = PostgreSQL") {
		t.Fatalf("unexpected exported text: %s", exported.String())
	}
}
