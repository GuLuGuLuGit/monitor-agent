//go:build !darwin

package identity

import (
	"fmt"
	"os"
	"path/filepath"
)

type fileStore struct {
	dir string
}

// NewFileStore returns a KeyStore backed by the filesystem.
// Private key is stored at {dir}/private.key with 0600 permission.
func NewFileStore(dir string) KeyStore {
	return &fileStore{dir: dir}
}

func (f *fileStore) keyPath(nodeID string) string {
	return filepath.Join(f.dir, "private.key")
}

func (f *fileStore) SavePrivateKey(nodeID string, keyPEM []byte) error {
	if err := os.MkdirAll(f.dir, 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return os.WriteFile(f.keyPath(nodeID), keyPEM, 0600)
}

func (f *fileStore) LoadPrivateKey(nodeID string) ([]byte, error) {
	data, err := os.ReadFile(f.keyPath(nodeID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file: key not found for node %s", nodeID)
		}
		return nil, err
	}
	return data, nil
}

func (f *fileStore) DeletePrivateKey(nodeID string) error {
	return os.Remove(f.keyPath(nodeID))
}

func (f *fileStore) StorageType() string {
	return "File"
}
