//go:build !darwin

package identity

func newPlatformKeyStore(dir string) KeyStore {
	return NewFileStore(dir)
}
