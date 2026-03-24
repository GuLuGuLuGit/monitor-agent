package identity

import (
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/uuid"
)

// Identity holds the node's persistent cryptographic identity.
type Identity struct {
	NodeID     string
	PublicKey  *rsa.PublicKey
	PrivateKey *rsa.PrivateKey
	store      KeyStore
	dataDir    string
}

// LoadOrCreate loads an existing identity from dataDir or generates a new one.
// On macOS the private key is read from / written to Keychain; on other
// platforms it falls back to a file.
func LoadOrCreate(dataDir string) (*Identity, error) {
	identityDir := filepath.Join(dataDir, "identity")
	if err := os.MkdirAll(identityDir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir identity dir: %w", err)
	}

	unlock, err := lockIdentityDir(identityDir)
	if err != nil {
		return nil, err
	}
	defer unlock()

	store := newPlatformKeyStore(identityDir)

	id := &Identity{
		store:   store,
		dataDir: dataDir,
	}

	// 1. Load or generate Node ID
	nodeID, err := id.loadOrCreateNodeID(identityDir)
	if err != nil {
		return nil, err
	}
	id.NodeID = nodeID

	// 2. Try loading existing key pair
	pubKeyPath := filepath.Join(identityDir, "public.pem")

	privPEM, privErr := store.LoadPrivateKey(nodeID)
	pubPEM, pubErr := os.ReadFile(pubKeyPath)

	if privErr == nil && pubErr == nil {
		priv, err := ParsePrivateKeyPEM(privPEM)
		if err != nil {
			return nil, fmt.Errorf("parse stored private key: %w", err)
		}
		pub, err := ParsePublicKeyPEM(pubPEM)
		if err != nil {
			return nil, fmt.Errorf("parse stored public key: %w", err)
		}
		if err := VerifyKeyPair(priv, pub); err != nil {
			// Mismatch — regenerate
			fmt.Printf("[!] Key pair mismatch detected, regenerating...\n")
		} else {
			id.PrivateKey = priv
			id.PublicKey = pub
			return id, nil
		}
	}

	// 3. Generate new key pair
	priv, err := GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate key pair: %w", err)
	}
	id.PrivateKey = priv
	id.PublicKey = &priv.PublicKey

	// Save private key to platform store
	privPEM = MarshalPrivateKeyPEM(priv)
	if err := store.SavePrivateKey(nodeID, privPEM); err != nil {
		return nil, fmt.Errorf("save private key: %w", err)
	}

	// Save public key to file (public information)
	pubPEM, err = MarshalPublicKeyPEM(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}
	if err := os.WriteFile(pubKeyPath, pubPEM, 0644); err != nil {
		return nil, fmt.Errorf("write public key file: %w", err)
	}

	return id, nil
}

func (id *Identity) loadOrCreateNodeID(identityDir string) (string, error) {
	path := filepath.Join(identityDir, "node_id.txt")

	data, err := os.ReadFile(path)
	if err == nil {
		nodeID := strings.TrimSpace(string(data))
		if nodeID != "" {
			return nodeID, nil
		}
	}

	nodeID := uuid.New().String()
	if err := os.WriteFile(path, []byte(nodeID+"\n"), 0644); err != nil {
		return "", fmt.Errorf("write node_id: %w", err)
	}
	return nodeID, nil
}

// PublicKeyPEM returns the PEM-encoded public key.
func (id *Identity) PublicKeyPEM() (string, error) {
	pem, err := MarshalPublicKeyPEM(id.PublicKey)
	if err != nil {
		return "", err
	}
	return string(pem), nil
}

// FingerprintStr returns the SHA-256 hex fingerprint of the public key.
func (id *Identity) FingerprintStr() (string, error) {
	return Fingerprint(id.PublicKey)
}

// StorageType returns where the private key is stored.
func (id *Identity) StorageType() string {
	return id.store.StorageType()
}

func lockIdentityDir(identityDir string) (func(), error) {
	lockPath := filepath.Join(identityDir, ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open identity lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock identity dir: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
