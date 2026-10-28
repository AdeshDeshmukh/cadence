package main

import (
	"fmt"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/audio"
)

func main() {
	fmt.Println("==================================================")
	fmt.Println("🎙️  Cadence Audio Capture & Pre-Roll Verification")
	fmt.Println("==================================================")

	engine := audio.NewEngine()
	devices, err := engine.Devices()
	if err != nil {
		fmt.Printf("⚠️  No input devices detected: %v\n", err)
	} else {
		fmt.Printf("🔍 Discovered %d audio input devices:\n", len(devices))
		for i, d := range devices {
			status := ""
			if d.Default {
				status = " [DEFAULT]"
			}
			fmt.Printf("   [%d] %s (%s, %d Hz)%s\n", i+1, d.Name, d.ID, d.SampleRate, status)
		}
	}

	selected, err := engine.DeviceManager().SelectedDevice()
	if err == nil {
		fmt.Printf("\n🎯 Active Input Device: %s\n", selected.Name)
	}

	fmt.Println("\n▶️  Starting 3-second live audio capture...")
	stream, err := engine.Start()
	if err != nil {
		fmt.Printf("❌ Failed to start audio capture: %v\n", err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	totalChunks := 0
	totalSamples := 0
	maxPeak := float32(0)

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()

captureLoop:
	for {
		select {
		case <-sigChan:
			fmt.Println("\nInterrupted by user.")
			break captureLoop
		case <-timer.C:
			fmt.Println("\n✅ 3-second test duration reached.")
			break captureLoop
		case chunk, ok := <-stream:
			if !ok {
				break captureLoop
			}
			totalChunks++
			totalSamples += len(chunk.Samples)

			// Compute RMS and Peak for visualization
			sumSquares := float64(0)
			for _, s := range chunk.Samples {
				abs := float32(math.Abs(float64(s)))
				if abs > maxPeak {
					maxPeak = abs
				}
				sumSquares += float64(s * s)
			}
			rms := math.Sqrt(sumSquares / float64(len(chunk.Samples)))
			db := 20 * math.Log10(math.Max(rms, 1e-5))

			bars := int((db + 60) / 60 * 20)
			if bars < 0 {
				bars = 0
			}
			if bars > 20 {
				bars = 20
			}
			meter := ""
			for b := 0; b < 20; b++ {
				if b < bars {
					meter += "█"
				} else {
					meter += "░"
				}
			}

			fmt.Printf("\r  [%s] RMS: %6.1f dB | Chunks: %-4d | Samples: %-7d", meter, db, totalChunks, totalSamples)
		}
	}

	if err := engine.Stop(); err != nil {
		fmt.Printf("\n⚠️ Stop returned: %v\n", err)
	}

	fmt.Println("\n\n📊 Audio Verification Summary:")
	fmt.Printf("   • Total Chunks Received:  %d\n", totalChunks)
	fmt.Printf("   • Total Samples Captured: %d (~%.2f seconds at 16kHz)\n", totalSamples, float64(totalSamples)/16000.0)
	fmt.Printf("   • Peak Audio Amplitude:   %.4f\n", maxPeak)
	fmt.Printf("   • Pre-Roll Ring Buffer:   %d samples stored\n", engine.RingBuffer().Len())
	fmt.Println("==================================================")
}
