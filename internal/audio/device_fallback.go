//go:build !darwin && !linux && !windows

package audio

func newPlatformEnumerator() DeviceEnumerator {
	return &FallbackEnumerator{}
}
