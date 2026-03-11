package identity

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

const rsaKeyBits = 2048

// GenerateKeyPair creates a new RSA-2048 key pair.
func GenerateKeyPair() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, rsaKeyBits)
}

// MarshalPrivateKeyPEM encodes a private key to PEM bytes.
func MarshalPrivateKeyPEM(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

// ParsePrivateKeyPEM decodes PEM bytes back to a private key.
func ParsePrivateKeyPEM(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// MarshalPublicKeyPEM encodes a public key to PEM bytes.
func MarshalPublicKeyPEM(key *rsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}), nil
}

// ParsePublicKeyPEM decodes PEM bytes back to a public key.
func ParsePublicKeyPEM(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	return rsaPub, nil
}

// Fingerprint returns the SHA-256 hex fingerprint of a public key.
func Fingerprint(key *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(der)
	return fmt.Sprintf("%x", hash), nil
}

// VerifyKeyPair checks that a private key matches a public key.
func VerifyKeyPair(priv *rsa.PrivateKey, pub *rsa.PublicKey) error {
	if priv.PublicKey.N.Cmp(pub.N) != 0 || priv.PublicKey.E != pub.E {
		return fmt.Errorf("public key does not match private key")
	}
	return nil
}
