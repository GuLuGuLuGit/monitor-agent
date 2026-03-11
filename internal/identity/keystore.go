package identity

// KeyStore abstracts private key storage across platforms.
// macOS uses Keychain; Linux/Docker uses the filesystem as a fallback.
type KeyStore interface {
	// SavePrivateKey persists a PEM-encoded private key for the given node.
	SavePrivateKey(nodeID string, keyPEM []byte) error

	// LoadPrivateKey retrieves the PEM-encoded private key, or returns
	// ErrKeyNotFound if no key is stored for this node.
	LoadPrivateKey(nodeID string) ([]byte, error)

	// DeletePrivateKey removes the stored key (used during uninstall).
	DeletePrivateKey(nodeID string) error

	// StorageType returns a human-readable label, e.g. "macOS Keychain" or "File".
	StorageType() string
}
