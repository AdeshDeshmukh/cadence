package benchmarks

import (
	"strings"
	"testing"
	"time"
)

func TestCalculatePercentiles(t *testing.T) {
	// Empty slice test
	empty := CalculatePercentiles(nil, time.Second)
	if empty.Count != 0 {
		t.Fatalf("expected count 0 for empty slice, got: %d", empty.Count)
	}

	samples := []time.Duration{
		100 * time.Millisecond,
		150 * time.Millisecond,
		200 * time.Millisecond,
		250 * time.Millisecond,
		300 * time.Millisecond,
		350 * time.Millisecond,
		400 * time.Millisecond,
		450 * time.Millisecond,
		500 * time.Millisecond,
		1000 * time.Millisecond,
	}

	res := CalculatePercentiles(samples, 1*time.Second)
	if res.Count != 10 {
		t.Fatalf("expected count 10, got: %d", res.Count)
	}
	if res.Min != 100*time.Millisecond {
		t.Errorf("expected min 100ms, got: %v", res.Min)
	}
	if res.Max != 1000*time.Millisecond {
		t.Errorf("expected max 1000ms, got: %v", res.Max)
	}
	if res.P50 != 300*time.Millisecond {
		t.Errorf("expected P50 300ms, got: %v", res.P50)
	}
	if res.RTF <= 0 {
		t.Errorf("expected positive real time factor, got: %f", res.RTF)
	}
}

func TestCalculateWER(t *testing.T) {
	// Perfect match
	wer, s, d, ins := CalculateWER("hello world", "hello world")
	if wer != 0.0 || s != 0 || d != 0 || ins != 0 {
		t.Errorf("expected perfect match 0 WER, got: %f, s=%d, d=%d, i=%d", wer, s, d, ins)
	}

	// 1 substitution
	werSub, sSub, _, _ := CalculateWER("hello world", "hello there")
	if werSub != 0.5 || sSub != 1 {
		t.Errorf("expected 0.5 WER for 1 substitution in 2 words, got: %f", werSub)
	}

	// 1 deletion
	werDel, _, dDel, _ := CalculateWER("the quick brown fox", "the brown fox")
	if werDel != 0.25 || dDel != 1 {
		t.Errorf("expected 0.25 WER for 1 deletion in 4 words, got: %f", werDel)
	}

	// 1 insertion
	werIns, _, _, iIns := CalculateWER("the fox", "the quick fox")
	if werIns != 0.5 || iIns != 1 {
		t.Errorf("expected 0.5 WER for 1 insertion, got: %f", werIns)
	}

	// Empty cases
	werEmpty, _, _, _ := CalculateWER("", "")
	if werEmpty != 0.0 {
		t.Errorf("expected 0 WER for empty strings, got: %f", werEmpty)
	}

	werRefEmpty, _, _, _ := CalculateWER("", "some text")
	if werRefEmpty != 1.0 {
		t.Errorf("expected 1.0 WER for empty reference, got: %f", werRefEmpty)
	}
}

func TestCalculateCER(t *testing.T) {
	cerSame := CalculateCER("hello", "hello")
	if cerSame != 0.0 {
		t.Errorf("expected 0.0 CER, got: %f", cerSame)
	}

	cerDiff := CalculateCER("cat", "cut")
	if cerDiff != 1.0/3.0 {
		t.Errorf("expected 1/3 CER, got: %f", cerDiff)
	}

	cerEmpty := CalculateCER("", "")
	if cerEmpty != 0.0 {
		t.Errorf("expected 0 CER for empty, got: %f", cerEmpty)
	}
}

func TestCaptureMemory(t *testing.T) {
	mem := CaptureMemory()
	if mem.AllocBytes == 0 {
		t.Errorf("expected non-zero AllocBytes")
	}
	if mem.SysMB <= 0 {
		t.Errorf("expected non-zero SysMB")
	}
}

func TestRunBenchmarkSuiteAndMarkdown(t *testing.T) {
	report := RunBenchmarkSuite(20)
	if report.Platform == "" {
		t.Errorf("expected platform to be populated")
	}
	if report.Latency1s.Count != 20 {
		t.Errorf("expected 20 samples in 1s latency, got: %d", report.Latency1s.Count)
	}

	md := FormatReportMarkdown(report)
	if !strings.Contains(md, "# Cadence Performance Benchmark Report") {
		t.Errorf("expected markdown header, got: %s", md)
	}
	if !strings.Contains(md, "Latency & Real-Time Factor") {
		t.Errorf("expected Latency section, got: %s", md)
	}
	if !strings.Contains(md, "Word Error Rate (WER)") {
		t.Errorf("expected WER section, got: %s", md)
	}
	if !strings.Contains(md, "Memory Consumption") {
		t.Errorf("expected Memory section, got: %s", md)
	}
}
