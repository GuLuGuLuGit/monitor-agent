//go:build darwin

package identity

func newPlatformKeyStore(_ string) KeyStore {
	return NewKeychainStore()
}
