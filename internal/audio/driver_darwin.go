//go:build darwin && cgo

package audio

/*
#cgo darwin LDFLAGS: -framework CoreAudio -framework AudioToolbox
#include <AudioToolbox/AudioToolbox.h>
#include <CoreAudio/CoreAudio.h>
#include <stdlib.h>
#include <string.h>

extern void cadenceAudioInputCallback(void *audioData, UInt32 byteSize);

static void cAudioInputCallback(
    void *inUserData,
    AudioQueueRef inAQ,
    AudioQueueBufferRef inBuffer,
    const AudioTimeStamp *inStartTime,
    UInt32 inNumPackets,
    const AudioStreamPacketDescription *inPacketDesc)
{
    if (inBuffer && inBuffer->mAudioDataByteSize > 0) {
        cadenceAudioInputCallback(inBuffer->mAudioData, inBuffer->mAudioDataByteSize);
    }
    AudioQueueEnqueueBuffer(inAQ, inBuffer, 0, NULL);
}

static OSStatus createAudioQueue(AudioQueueRef *outAQ, Float64 sampleRate) {
    AudioStreamBasicDescription format;
    memset(&format, 0, sizeof(format));
    format.mSampleRate = sampleRate;
    format.mFormatID = kAudioFormatLinearPCM;
    format.mFormatFlags = kAudioFormatFlagIsFloat | kAudioFormatFlagIsPacked;
    format.mFramesPerPacket = 1;
    format.mChannelsPerFrame = 1; // mono
    format.mBitsPerChannel = 32;  // float32
    format.mBytesPerPacket = 4;
    format.mBytesPerFrame = 4;

    return AudioQueueNewInput(&format, cAudioInputCallback, NULL, NULL, NULL, 0, outAQ);
}
*/
import "C"

import (
	"context"
	"errors"
	"sync"
	"time"
	"unsafe"
)

// Active stream registry to route C callbacks back to the active Go driver instance
var (
	registryMu sync.RWMutex
	activeAQ   *DarwinAudioDriver
)

// DarwinAudioDriver captures live microphone input via macOS CoreAudio AudioQueue.
type DarwinAudioDriver struct {
	mu         sync.Mutex
	queue      C.AudioQueueRef
	buffers    [3]C.AudioQueueBufferRef
	outChan    chan<- []float32
	ctx        context.Context
	isRunning  bool
	sampleRate int
}

//export cadenceAudioInputCallback
func cadenceAudioInputCallback(audioData unsafe.Pointer, byteSize C.UInt32) {
	registryMu.RLock()
	driver := activeAQ
	registryMu.RUnlock()

	if driver == nil || !driver.isRunning || audioData == nil {
		return
	}

	numBytes := int(byteSize)
	numSamples := numBytes / 4 // 4 bytes per float32
	if numSamples > 0 {
		src := unsafe.Slice((*float32)(audioData), numSamples)
		samples := make([]float32, numSamples)
		copy(samples, src)

		select {
		case <-driver.ctx.Done():
			return
		case driver.outChan <- samples:
		default:
			// Non-blocking drop on saturated channel to preserve real-time low latency
		}
	}
}

// NewDarwinAudioDriver creates a CoreAudio driver instance.
func NewDarwinAudioDriver() *DarwinAudioDriver {
	return &DarwinAudioDriver{}
}

// StartStream initializes AudioQueue buffers and starts capturing microphone audio.
func (d *DarwinAudioDriver) StartStream(ctx context.Context, sampleRate int, out chan<- []float32) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isRunning {
		return errors.New("audio: darwin audio driver already running")
	}

	if sampleRate <= 0 {
		sampleRate = DefaultSampleRate
	}

	d.ctx = ctx
	d.outChan = out
	d.sampleRate = sampleRate

	var aq C.AudioQueueRef
	status := C.createAudioQueue(&aq, C.Float64(sampleRate))
	if status != 0 {
		return errors.New("audio: failed to initialize macOS AudioQueue (check mic permissions)")
	}
	d.queue = aq

	// Allocate 3 circular buffers for 50ms chunks (e.g. 800 samples at 16kHz = 3200 bytes)
	bufferByteSize := C.UInt32(sampleRate * 4 / 20)
	if bufferByteSize < 1024 {
		bufferByteSize = 1024
	}

	for i := 0; i < 3; i++ {
		st := C.AudioQueueAllocateBuffer(d.queue, bufferByteSize, &d.buffers[i])
		if st != 0 {
			C.AudioQueueDispose(d.queue, C.Boolean(1))
			return errors.New("audio: failed to allocate AudioQueue buffers")
		}
		C.AudioQueueEnqueueBuffer(d.queue, d.buffers[i], 0, nil)
	}

	registryMu.Lock()
	activeAQ = d
	registryMu.Unlock()

	startStatus := C.AudioQueueStart(d.queue, nil)
	if startStatus != 0 {
		C.AudioQueueDispose(d.queue, C.Boolean(1))
		registryMu.Lock()
		activeAQ = nil
		registryMu.Unlock()
		return errors.New("audio: failed to start AudioQueue capture")
	}

	d.isRunning = true
	return nil
}

// StopStream stops AudioQueue and releases buffers.
func (d *DarwinAudioDriver) StopStream() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isRunning {
		return nil
	}

	d.isRunning = false
	registryMu.Lock()
	activeAQ = nil
	registryMu.Unlock()

	if d.queue != nil {
		C.AudioQueueStop(d.queue, C.Boolean(1))
		C.AudioQueueDispose(d.queue, C.Boolean(1))
		d.queue = nil
	}

	return nil
}

// NewDefaultStreamDriver returns the native DarwinAudioDriver, or SimulatedDriver fallback.
func NewDefaultStreamDriver() StreamDriver {
	driver := NewDarwinAudioDriver()
	var testAQ C.AudioQueueRef
	st := C.createAudioQueue(&testAQ, 16000)
	if st == 0 {
		C.AudioQueueDispose(testAQ, C.Boolean(1))
		return driver
	}
	// Fallback if mic permission not granted in headless/CI
	return NewSimulatedDriver(440, 512, 32*time.Millisecond)
}
