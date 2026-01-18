package crypto

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNewEncryptor(t *testing.T) {
	tests := []struct {
		name    string
		keyLen  int
		wantErr error
	}{
		{"valid 32-byte key", 32, nil},
		{"too short key", 16, ErrInvalidKey},
		{"too long key", 64, ErrInvalidKey},
		{"empty key", 0, ErrInvalidKey},
		{"31 bytes", 31, ErrInvalidKey},
		{"33 bytes", 33, ErrInvalidKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keyLen)
			for i := range key {
				key[i] = byte(i % 256)
			}

			enc, err := NewEncryptor(key)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("NewEncryptor() error = %v, want %v", err, tt.wantErr)
				}
				if enc != nil {
					t.Error("NewEncryptor() returned non-nil encryptor on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewEncryptor() unexpected error = %v", err)
				}
				if enc == nil {
					t.Error("NewEncryptor() returned nil encryptor")
				}
			}
		})
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{"simple text", "hello world"},
		{"empty string", ""},
		{"unicode text", "こんにちは世界 🔐"},
		{"long text", strings.Repeat("a", 10000)},
		{"special chars", "!@#$%^&*()_+-=[]{}|;':\",./<>?"},
		{"newlines", "line1\nline2\r\nline3"},
		{"API key format", "sk-proj-abc123def456ghi789"},
		{"JSON data", `{"key": "value", "nested": {"foo": "bar"}}`},
		{"binary-like", "\x00\x01\x02\x03\xff\xfe\xfd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := enc.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			// Empty string returns empty string
			if tt.plaintext == "" {
				if ciphertext != "" {
					t.Errorf("Encrypt() of empty string = %q, want empty", ciphertext)
				}
				return
			}

			// Verify ciphertext is valid base64
			if _, err := base64.StdEncoding.DecodeString(ciphertext); err != nil {
				t.Errorf("Encrypt() output is not valid base64: %v", err)
			}

			// Verify ciphertext is different from plaintext
			if ciphertext == tt.plaintext {
				t.Error("Encrypt() ciphertext equals plaintext")
			}

			// Decrypt
			decrypted, err := enc.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("Decrypt() = %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptProducesUniqueCiphertexts(t *testing.T) {
	key, _ := GenerateKey()
	enc, _ := NewEncryptor(key)

	plaintext := "same message"
	ciphertexts := make(map[string]bool)

	// Encrypt the same message 100 times
	for i := 0; i < 100; i++ {
		ct, err := enc.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt() error = %v", err)
		}

		if ciphertexts[ct] {
			t.Fatal("Encrypt() produced duplicate ciphertext - nonce reuse detected")
		}
		ciphertexts[ct] = true
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1, _ := GenerateKey()
	key2, _ := GenerateKey()

	enc1, _ := NewEncryptor(key1)
	enc2, _ := NewEncryptor(key2)

	plaintext := "secret message"
	ciphertext, _ := enc1.Encrypt(plaintext)

	// Attempt to decrypt with different key
	_, err := enc2.Decrypt(ciphertext)
	if err == nil {
		t.Error("Decrypt() with wrong key should fail")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key, _ := GenerateKey()
	enc, _ := NewEncryptor(key)

	plaintext := "secret message"
	ciphertext, _ := enc.Encrypt(plaintext)

	// Decode, tamper, re-encode
	data, _ := base64.StdEncoding.DecodeString(ciphertext)

	tests := []struct {
		name   string
		tamper func([]byte) []byte
	}{
		{
			"flip bit in nonce",
			func(d []byte) []byte {
				d[0] ^= 0x01
				return d
			},
		},
		{
			"flip bit in ciphertext",
			func(d []byte) []byte {
				d[len(d)/2] ^= 0x01
				return d
			},
		},
		{
			"flip bit in auth tag",
			func(d []byte) []byte {
				d[len(d)-1] ^= 0x01
				return d
			},
		},
		{
			"truncate",
			func(d []byte) []byte {
				return d[:len(d)-5]
			},
		},
		{
			"append garbage",
			func(d []byte) []byte {
				return append(d, 0x00, 0x01, 0x02)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to tamper with
			tampered := make([]byte, len(data))
			copy(tampered, data)
			tampered = tt.tamper(tampered)

			tamperedCT := base64.StdEncoding.EncodeToString(tampered)

			_, err := enc.Decrypt(tamperedCT)
			if err == nil {
				t.Error("Decrypt() of tampered ciphertext should fail")
			}
		})
	}
}

func TestDecryptInvalidInput(t *testing.T) {
	key, _ := GenerateKey()
	enc, _ := NewEncryptor(key)

	tests := []struct {
		name       string
		ciphertext string
		wantErr    bool
	}{
		{"empty string", "", false}, // Empty returns empty
		{"invalid base64", "not-valid-base64!!!", true},
		{"too short", base64.StdEncoding.EncodeToString([]byte("x")), true},
		{"just nonce", base64.StdEncoding.EncodeToString(make([]byte, 12)), true},
		{"random garbage", base64.StdEncoding.EncodeToString([]byte("random garbage data")), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := enc.Decrypt(tt.ciphertext)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Decrypt(%q) should have failed", tt.ciphertext)
				}
			} else {
				if err != nil {
					t.Errorf("Decrypt(%q) unexpected error = %v", tt.ciphertext, err)
				}
				if tt.ciphertext == "" && result != "" {
					t.Errorf("Decrypt(\"\") = %q, want \"\"", result)
				}
			}
		})
	}
}

func TestGenerateKey(t *testing.T) {
	keys := make(map[string]bool)

	for i := 0; i < 100; i++ {
		key, err := GenerateKey()
		if err != nil {
			t.Fatalf("GenerateKey() error = %v", err)
		}

		if len(key) != 32 {
			t.Errorf("GenerateKey() length = %d, want 32", len(key))
		}

		keyStr := string(key)
		if keys[keyStr] {
			t.Fatal("GenerateKey() produced duplicate key")
		}
		keys[keyStr] = true

		// Verify key can be used to create encryptor
		enc, err := NewEncryptor(key)
		if err != nil {
			t.Errorf("Generated key rejected by NewEncryptor: %v", err)
		}
		if enc == nil {
			t.Error("NewEncryptor returned nil for generated key")
		}
	}
}

func TestDeriveKeyFromSecret(t *testing.T) {
	tests := []struct {
		name   string
		secret string
	}{
		{"short secret", "abc"},
		{"exact length", strings.Repeat("x", 32)},
		{"long secret", strings.Repeat("y", 100)},
		{"empty secret", ""},
		{"unicode secret", "пароль密码"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := DeriveKeyFromSecret(tt.secret)

			if len(key) != 32 {
				t.Errorf("DeriveKeyFromSecret() length = %d, want 32", len(key))
			}

			// Verify key can be used
			enc, err := NewEncryptor(key)
			if err != nil {
				t.Errorf("Derived key rejected by NewEncryptor: %v", err)
			}

			// Verify deterministic
			key2 := DeriveKeyFromSecret(tt.secret)
			if string(key) != string(key2) {
				t.Error("DeriveKeyFromSecret() not deterministic")
			}

			// Verify can encrypt/decrypt
			if enc != nil {
				ct, _ := enc.Encrypt("test")
				pt, err := enc.Decrypt(ct)
				if err != nil || pt != "test" {
					t.Error("Derived key doesn't work for encrypt/decrypt")
				}
			}
		})
	}

	// Different secrets should produce different keys
	key1 := DeriveKeyFromSecret("secret1")
	key2 := DeriveKeyFromSecret("secret2")
	if string(key1) == string(key2) {
		t.Error("Different secrets produced same key")
	}
}

func TestConcurrentEncryptDecrypt(t *testing.T) {
	key, _ := GenerateKey()
	enc, _ := NewEncryptor(key)

	done := make(chan bool)
	errors := make(chan error, 100)

	// Run 10 goroutines each doing 100 encrypt/decrypt cycles
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				plaintext := strings.Repeat("x", id*10+j)
				ct, err := enc.Encrypt(plaintext)
				if err != nil {
					errors <- err
					done <- true
					return
				}
				pt, err := enc.Decrypt(ct)
				if err != nil {
					errors <- err
					done <- true
					return
				}
				if pt != plaintext {
					errors <- err
					done <- true
					return
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	select {
	case err := <-errors:
		t.Fatalf("Concurrent operation failed: %v", err)
	default:
		// Success
	}
}

// Benchmark tests
func BenchmarkEncrypt(b *testing.B) {
	key, _ := GenerateKey()
	enc, _ := NewEncryptor(key)
	plaintext := strings.Repeat("x", 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = enc.Encrypt(plaintext)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	key, _ := GenerateKey()
	enc, _ := NewEncryptor(key)
	ciphertext, _ := enc.Encrypt(strings.Repeat("x", 1000))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = enc.Decrypt(ciphertext)
	}
}
