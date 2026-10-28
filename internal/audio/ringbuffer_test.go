package audio

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestNewRingBuffer(t *testing.T) {
	rb := NewRingBuffer(100)
	if rb.Capacity() != 100 {
		t.Fatalf("expected capacity 100, got %d", rb.Capacity())
	}
	if rb.Len() != 0 {
		t.Fatalf("expected len 0, got %d", rb.Len())
	}

	// Default fallback
	rbDefault := NewRingBuffer(0)
	if rbDefault.Capacity() != DefaultSampleRate {
		t.Fatalf("expected default capacity %d, got %d", DefaultSampleRate, rbDefault.Capacity())
	}
}

func TestNewPreRollBuffer(t *testing.T) {
	rb := NewPreRollBuffer(500*time.Millisecond, 16000)
	expectedCapacity := 8000 // 0.5s * 16000
	if rb.Capacity() != expectedCapacity {
		t.Fatalf("expected capacity %d, got %d", expectedCapacity, rb.Capacity())
	}

	// Fallback check
	rbDefault := NewPreRollBuffer(0, 0)
	expectedDefault := int(DefaultPreRollDuration.Seconds() * float64(DefaultSampleRate))
	if rbDefault.Capacity() != expectedDefault {
		t.Fatalf("expected default capacity %d, got %d", expectedDefault, rbDefault.Capacity())
	}

	small := NewPreRollBuffer(time.Nanosecond, 2)
	if small.Capacity() != 1 {
		t.Fatalf("expected small-duration fallback capacity 1, got %d", small.Capacity())
	}
}

func TestRingBuffer_Write_Read_Basic(t *testing.T) {
	rb := NewRingBuffer(10)
	samples := []float32{1.0, 2.0, 3.0, 4.0, 5.0}

	n := rb.Write(samples)
	if n != 5 {
		t.Fatalf("expected write 5, got %d", n)
	}
	if rb.Len() != 5 {
		t.Fatalf("expected len 5, got %d", rb.Len())
	}

	out := make([]float32, 3)
	readN := rb.Read(out)
	if readN != 3 {
		t.Fatalf("expected read 3, got %d", readN)
	}
	expectedRead := []float32{1.0, 2.0, 3.0}
	if !reflect.DeepEqual(out, expectedRead) {
		t.Fatalf("expected read %v, got %v", expectedRead, out)
	}
	if rb.Len() != 2 {
		t.Fatalf("expected remaining len 2, got %d", rb.Len())
	}

	// Read remaining
	remaining := rb.ReadAll()
	expectedRemaining := []float32{4.0, 5.0}
	if !reflect.DeepEqual(remaining, expectedRemaining) {
		t.Fatalf("expected remaining %v, got %v", expectedRemaining, remaining)
	}
	if rb.Len() != 0 {
		t.Fatalf("expected len 0 after ReadAll, got %d", rb.Len())
	}
}

func TestRingBuffer_Overflow_Overwrite(t *testing.T) {
	rb := NewRingBuffer(5)

	// Write 4 samples: [1, 2, 3, 4]
	rb.Write([]float32{1, 2, 3, 4})
	if rb.Len() != 4 {
		t.Fatalf("expected len 4, got %d", rb.Len())
	}

	// Write 3 more samples: [5, 6, 7] -> causes 2 samples to overwrite [1, 2]
	// Remaining in FIFO should be: [3, 4, 5, 6, 7]
	rb.Write([]float32{5, 6, 7})
	if rb.Len() != 5 {
		t.Fatalf("expected len 5 after overflow, got %d", rb.Len())
	}

	all := rb.ReadAll()
	expected := []float32{3, 4, 5, 6, 7}
	if !reflect.DeepEqual(all, expected) {
		t.Fatalf("expected %v after overwrite, got %v", expected, all)
	}
}

func TestRingBuffer_Write_GreaterThanCapacity(t *testing.T) {
	rb := NewRingBuffer(4)

	// Write 7 items directly
	samples := []float32{1, 2, 3, 4, 5, 6, 7}
	rb.Write(samples)

	if rb.Len() != 4 {
		t.Fatalf("expected len 4, got %d", rb.Len())
	}

	all := rb.ReadAll()
	expected := []float32{4, 5, 6, 7}
	if !reflect.DeepEqual(all, expected) {
		t.Fatalf("expected %v, got %v", expected, all)
	}
}

func TestRingBuffer_ReadLatest(t *testing.T) {
	rb := NewRingBuffer(5)

	// Empty read
	if res := rb.ReadLatest(3); res != nil {
		t.Fatalf("expected nil for empty read, got %v", res)
	}
	if res := rb.ReadLatest(0); res != nil {
		t.Fatalf("expected nil for non-positive read, got %v", res)
	}

	// Fill partially
	rb.Write([]float32{10, 20, 30})
	latest := rb.ReadLatest(2)
	expected := []float32{20, 30}
	if !reflect.DeepEqual(latest, expected) {
		t.Fatalf("expected %v, got %v", expected, latest)
	}

	// Request more than available
	latestAll := rb.ReadLatest(10)
	expectedAll := []float32{10, 20, 30}
	if !reflect.DeepEqual(latestAll, expectedAll) {
		t.Fatalf("expected %v, got %v", expectedAll, latestAll)
	}

	// Cause circular wrap-around
	rb.Write([]float32{40, 50, 60}) // Buffer is now [20, 30, 40, 50, 60]
	latestWrap := rb.ReadLatest(3)
	expectedWrap := []float32{40, 50, 60}
	if !reflect.DeepEqual(latestWrap, expectedWrap) {
		t.Fatalf("expected %v, got %v", expectedWrap, latestWrap)
	}

	// Full latest
	latestFull := rb.ReadLatest(5)
	expectedFull := []float32{20, 30, 40, 50, 60}
	if !reflect.DeepEqual(latestFull, expectedFull) {
		t.Fatalf("expected %v, got %v", expectedFull, latestFull)
	}
}

func TestRingBuffer_ReadLatestDuration(t *testing.T) {
	rb := NewRingBuffer(16000)
	samples := make([]float32, 16000)
	for i := range samples {
		samples[i] = float32(i)
	}
	rb.Write(samples)

	// 250ms at 16000Hz = 4000 samples
	res := rb.ReadLatestDuration(250*time.Millisecond, 16000)
	if len(res) != 4000 {
		t.Fatalf("expected 4000 samples, got %d", len(res))
	}
	if res[0] != 12000 || res[3999] != 15999 {
		t.Fatalf("unexpected boundaries: first=%f, last=%f", res[0], res[3999])
	}

	// Invalid parameters
	if rb.ReadLatestDuration(0, 16000) != nil {
		t.Fatalf("expected nil for 0 duration")
	}
	if rb.ReadLatestDuration(100*time.Millisecond, 0) != nil {
		t.Fatalf("expected nil for 0 sample rate")
	}
}

func TestRingBuffer_Peek_Reset(t *testing.T) {
	rb := NewRingBuffer(5)
	rb.Write([]float32{1, 2, 3})

	peeked := rb.Peek(2)
	expected := []float32{1, 2}
	if !reflect.DeepEqual(peeked, expected) {
		t.Fatalf("expected %v, got %v", expected, peeked)
	}
	if got := rb.Peek(10); !reflect.DeepEqual(got, []float32{1, 2, 3}) {
		t.Fatalf("expected all available samples, got %v", got)
	}

	wrapped := NewRingBuffer(5)
	wrapped.Write([]float32{1, 2, 3, 4})
	wrapped.Read(make([]float32, 3))
	wrapped.Write([]float32{5, 6, 7})
	if got := wrapped.Peek(4); !reflect.DeepEqual(got, []float32{4, 5, 6, 7}) {
		t.Fatalf("expected wrapped samples, got %v", got)
	}

	// Peek should not change length
	if rb.Len() != 3 {
		t.Fatalf("expected len 3, got %d", rb.Len())
	}

	if rb.Peek(0) != nil {
		t.Fatalf("expected nil for peek <= 0")
	}

	rb.Reset()
	if rb.Len() != 0 {
		t.Fatalf("expected len 0 after Reset, got %d", rb.Len())
	}
	if rb.Peek(1) != nil {
		t.Fatalf("expected nil peek from empty buffer")
	}
	if rb.ReadAll() != nil {
		t.Fatalf("expected nil from ReadAll on empty buffer")
	}
}

func TestRingBuffer_WriteChunk(t *testing.T) {
	rb := NewRingBuffer(10)
	chunk := Chunk{
		Samples:    []float32{0.1, 0.2, 0.3},
		SampleRate: 16000,
		Timestamp:  time.Now(),
	}

	n := rb.WriteChunk(chunk)
	if n != 3 {
		t.Fatalf("expected 3 samples written, got %d", n)
	}
	if rb.Len() != 3 {
		t.Fatalf("expected len 3, got %d", rb.Len())
	}

	// Empty write
	if rb.Write(nil) != 0 {
		t.Fatalf("expected 0 written for nil")
	}
	if rb.Read(nil) != 0 {
		t.Fatalf("expected 0 read for nil")
	}
}

func TestRingBuffer_ConcurrentRace(t *testing.T) {
	rb := NewRingBuffer(1000)
	var wg sync.WaitGroup

	numWriters := 4
	numReaders := 4
	iterations := 200

	// Concurrent writers
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			samples := []float32{float32(id), float32(id) + 0.1, float32(id) + 0.2}
			for i := 0; i < iterations; i++ {
				rb.Write(samples)
				time.Sleep(10 * time.Microsecond)
			}
		}(w)
	}

	// Concurrent readers (ReadLatest & Read)
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]float32, 10)
			for i := 0; i < iterations; i++ {
				_ = rb.ReadLatest(50)
				_ = rb.Read(buf)
				_ = rb.Len()
				time.Sleep(10 * time.Microsecond)
			}
		}()
	}

	wg.Wait()
}

func BenchmarkRingBuffer_Write(b *testing.B) {
	rb := NewRingBuffer(16000)
	samples := make([]float32, 512)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rb.Write(samples)
	}
}

func BenchmarkRingBuffer_ReadLatest(b *testing.B) {
	rb := NewRingBuffer(16000)
	samples := make([]float32, 16000)
	rb.Write(samples)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = rb.ReadLatest(8000)
	}
}
