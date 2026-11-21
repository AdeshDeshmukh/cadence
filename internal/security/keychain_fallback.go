//go:build !darwin && !linux && !windows

package security

func newPlatformSecretStore() SecretStore {
	return NewMemoryStore()
}
