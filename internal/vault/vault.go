package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidKey    = errors.New("vault: invalid master key, must be 32 bytes (64 hex chars)")
	ErrDecryptFailed = errors.New("vault: decryption failed, data may be corrupted or wrong key")
	ErrEmptyData     = errors.New("vault: cannot encrypt empty data")
)

// Vault handles AES-256-GCM encryption/decryption for secure storage
type Vault struct {
	key []byte
	gcm cipher.AEAD
}

// NewVault creates a new Vault with the given hex-encoded master key
// The key must be 64 hex characters (32 bytes for AES-256)
func NewVault(hexKey string) (*Vault, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil || len(key) != 32 {
		return nil, ErrInvalidKey
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("vault: failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("vault: failed to create GCM: %w", err)
	}

	return &Vault{key: key, gcm: gcm}, nil
}

// Encrypt encrypts plaintext data and returns (ciphertext, nonce, error)
// Each encryption uses a unique random nonce
func (v *Vault) Encrypt(plaintext []byte) (ciphertext []byte, nonce []byte, err error) {
	if len(plaintext) == 0 {
		return nil, nil, ErrEmptyData
	}

	nonce = make([]byte, v.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("vault: failed to generate nonce: %w", err)
	}

	ciphertext = v.gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt decrypts ciphertext using the given nonce
func (v *Vault) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	plaintext, err := v.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	return plaintext, nil
}

// GenerateMasterKey generates a random 32-byte key and returns it hex-encoded
// Use this to generate a new master key for first-time setup
func GenerateMasterKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", fmt.Errorf("vault: failed to generate key: %w", err)
	}
	return hex.EncodeToString(key), nil
}
