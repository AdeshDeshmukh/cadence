//go:build !darwin && !linux

package audio

func newPlatformEnumerator() DeviceEnumerator {
	return &FallbackEnumerator{}
}
