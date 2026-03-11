//go:build darwin

package identity

import (
	"fmt"

	"github.com/keybase/go-keychain"
)

const keychainService = "com.openclaw.agent"

type keychainStore struct{}

// NewKeychainStore returns a KeyStore backed by macOS Keychain.
func NewKeychainStore() KeyStore {
	return &keychainStore{}
}

func (k *keychainStore) SavePrivateKey(nodeID string, keyPEM []byte) error {
	// Delete any existing item first to avoid duplicates.
	_ = k.DeletePrivateKey(nodeID)

	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(keychainService)
	item.SetAccount(nodeID)
	item.SetLabel("OpenClaw Agent Private Key")
	item.SetData(keyPEM)
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlocked)

	if err := keychain.AddItem(item); err != nil {
		return fmt.Errorf("keychain add: %w", err)
	}
	return nil
}

func (k *keychainStore) LoadPrivateKey(nodeID string) ([]byte, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(keychainService)
	query.SetAccount(nodeID)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)

	results, err := keychain.QueryItem(query)
	if err != nil {
		return nil, fmt.Errorf("keychain query: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("keychain: key not found for node %s", nodeID)
	}
	return results[0].Data, nil
}

func (k *keychainStore) DeletePrivateKey(nodeID string) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(keychainService)
	item.SetAccount(nodeID)

	return keychain.DeleteItem(item)
}

func (k *keychainStore) StorageType() string {
	return "macOS Keychain"
}
