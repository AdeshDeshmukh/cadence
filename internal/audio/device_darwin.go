//go:build darwin

package audio

func newPlatformEnumerator() DeviceEnumerator {
	return &DarwinAudioEnumerator{}
}
