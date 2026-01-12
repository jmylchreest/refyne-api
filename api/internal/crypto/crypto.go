package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

var (
	ErrInvalidKey    = errors.New("encryption key must be 32 bytes for AES-256")
	ErrInvalidCipher = errors.New("invalid ciphertext")
)

// Encryptor provides AES-256-GCM encryption for sensitive data.
type Encryptor struct {
	gcm cipher.AEAD
}

// NewEncryptor creates a new Encryptor with the given key.
// The key must be exactly 32 bytes for AES-256.
func NewEncryptor(key []byte) (*Encryptor, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &Encryptor{gcm: gcm}, nil
}

// Encrypt encrypts plaintext and returns base64-encoded ciphertext.
// The output format is: base64(nonce || ciphertext || tag)
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	// Generate random nonce
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt (GCM appends authentication tag automatically)
	ciphertext := e.gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode as base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded ciphertext and returns plaintext.
func (e *Encryptor) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	// Decode base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	// Validate minimum length (nonce + at least 1 byte + tag)
	nonceSize := e.gcm.NonceSize()
	if len(data) < nonceSize+1 {
		return "", ErrInvalidCipher
	}

	// Extract nonce and ciphertext
	nonce, cipherData := data[:nonceSize], data[nonceSize:]

	// Decrypt and verify
	plaintext, err := e.gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	return string(plaintext), nil
}

// GenerateKey generates a random 32-byte key for AES-256.
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return key, nil
}

// DeriveKey derives a 32-byte key from a password using a simple approach.
// For production, consider using a proper KDF like Argon2 or scrypt.
// This is a simplified version that pads/truncates the input to 32 bytes.
func DeriveKeyFromSecret(secret string) []byte {
	key := make([]byte, 32)
	secretBytes := []byte(secret)

	// Copy secret bytes into key, cycling if necessary
	for i := 0; i < 32; i++ {
		if len(secretBytes) > 0 {
			key[i] = secretBytes[i%len(secretBytes)]
		}
	}

	return key
}
