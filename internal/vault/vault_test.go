package vault

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	// Generate a test key
	key := make([]byte, 32)
	rand.Read(key)
	hexKey := hex.EncodeToString(key)

	v, err := NewVault(hexKey)
	if err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	plaintext := []byte("my-secret-api-key-12345")

	ciphertext, nonce, err := v.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	if string(ciphertext) == string(plaintext) {
		t.Fatal("ciphertext should not equal plaintext")
	}

	decrypted, err := v.Decrypt(ciphertext, nonce)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted text doesn't match: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecryptLargeData(t *testing.T) {
	key, _ := GenerateMasterKey()
	v, err := NewVault(key)
	if err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	// Test with larger data (simulating a JSON blob with identity info)
	plaintext := []byte(`{"id_number": "110101199001011234", "bank_card": "6222021234567890123", "passport": "E12345678"}`)

	ciphertext, nonce, err := v.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	decrypted, err := v.Decrypt(ciphertext, nonce)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatal("decrypted data doesn't match original")
	}
}

func TestInvalidKey(t *testing.T) {
	_, err := NewVault("invalid")
	if err != ErrInvalidKey {
		t.Fatalf("expected ErrInvalidKey, got: %v", err)
	}

	_, err = NewVault("abcd") // too short
	if err != ErrInvalidKey {
		t.Fatalf("expected ErrInvalidKey, got: %v", err)
	}
}

func TestEmptyData(t *testing.T) {
	key, _ := GenerateMasterKey()
	v, _ := NewVault(key)

	_, _, err := v.Encrypt([]byte{})
	if err != ErrEmptyData {
		t.Fatalf("expected ErrEmptyData, got: %v", err)
	}
}

func TestWrongNonce(t *testing.T) {
	key, _ := GenerateMasterKey()
	v, _ := NewVault(key)

	ciphertext, _, _ := v.Encrypt([]byte("secret"))

	wrongNonce := make([]byte, v.gcm.NonceSize())
	_, err := v.Decrypt(ciphertext, wrongNonce)
	if err != ErrDecryptFailed {
		t.Fatalf("expected ErrDecryptFailed, got: %v", err)
	}
}

func TestGenerateMasterKey(t *testing.T) {
	key, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	decoded, err := hex.DecodeString(key)
	if err != nil {
		t.Fatalf("key is not valid hex: %v", err)
	}

	if len(decoded) != 32 {
		t.Fatalf("key should be 32 bytes, got %d", len(decoded))
	}

	// Generate another key, should be different
	key2, _ := GenerateMasterKey()
	if key == key2 {
		t.Fatal("two generated keys should not be identical")
	}
}

func TestUniqueNonces(t *testing.T) {
	key, _ := GenerateMasterKey()
	v, _ := NewVault(key)

	_, nonce1, _ := v.Encrypt([]byte("same data"))
	_, nonce2, _ := v.Encrypt([]byte("same data"))

	if hex.EncodeToString(nonce1) == hex.EncodeToString(nonce2) {
		t.Fatal("nonces should be unique for each encryption")
	}
}
