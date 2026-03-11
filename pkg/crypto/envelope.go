package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Envelope is the hybrid-encrypted message format.
// The session key is RSA-OAEP encrypted; the payload is AES-256-GCM encrypted.
type Envelope struct {
	Version      int    `json:"version"`
	EncryptedKey string `json:"encrypted_key"`
	Nonce        string `json:"nonce"`
	Ciphertext   string `json:"ciphertext"`
	Timestamp    int64  `json:"timestamp"`
}

// Seal encrypts plaintext using a random AES-256 session key,
// then encrypts the session key with the recipient's RSA public key (OAEP SHA-256).
func Seal(pub *rsa.PublicKey, plaintext []byte) (*Envelope, error) {
	sessionKey := make([]byte, 32)
	if _, err := rand.Read(sessionKey); err != nil {
		return nil, fmt.Errorf("generate session key: %w", err)
	}

	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes-gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, sessionKey, nil)
	if err != nil {
		return nil, fmt.Errorf("rsa encrypt session key: %w", err)
	}

	return &Envelope{
		Version:      1,
		EncryptedKey: base64.StdEncoding.EncodeToString(encryptedKey),
		Nonce:        base64.StdEncoding.EncodeToString(nonce),
		Ciphertext:   base64.StdEncoding.EncodeToString(ciphertext),
		Timestamp:    time.Now().Unix(),
	}, nil
}

// MaxEnvelopeAge limits how old an envelope can be to prevent replay attacks.
const MaxEnvelopeAge = 5 * time.Minute

// Open decrypts an Envelope using the recipient's RSA private key.
// Rejects envelopes older than MaxEnvelopeAge to mitigate replay attacks.
func Open(priv *rsa.PrivateKey, env *Envelope) ([]byte, error) {
	if env.Version != 1 {
		return nil, fmt.Errorf("unsupported envelope version: %d", env.Version)
	}

	if env.Timestamp > 0 {
		age := time.Since(time.Unix(env.Timestamp, 0))
		if age > MaxEnvelopeAge {
			return nil, fmt.Errorf("envelope expired: age %v exceeds max %v", age, MaxEnvelopeAge)
		}
		if age < -1*time.Minute {
			return nil, fmt.Errorf("envelope timestamp is in the future")
		}
	}

	encryptedKey, err := base64.StdEncoding.DecodeString(env.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted_key: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	sessionKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, encryptedKey, nil)
	if err != nil {
		return nil, fmt.Errorf("rsa decrypt session key: %w", err)
	}

	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes-gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("aes-gcm decrypt: %w", err)
	}

	return plaintext, nil
}

// SealJSON is a convenience wrapper: marshals v to JSON, then Seal.
func SealJSON(pub *rsa.PublicKey, v interface{}) (*Envelope, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal plaintext: %w", err)
	}
	return Seal(pub, data)
}

// OpenJSON is a convenience wrapper: Open, then unmarshal JSON into v.
func OpenJSON(priv *rsa.PrivateKey, env *Envelope, v interface{}) error {
	plaintext, err := Open(priv, env)
	if err != nil {
		return err
	}
	return json.Unmarshal(plaintext, v)
}
