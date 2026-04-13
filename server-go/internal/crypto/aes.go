package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/scrypt"
)

type EncryptedPayload struct {
	EncryptedData string `json:"encryptedData"`
	IV            string `json:"iv"`
	AuthTag       string `json:"authTag"`
	Algorithm     string `json:"algorithm"`
}

const (
	genericIVSize = 12
	casIVSize     = 16
	aesKeySize    = 32
	gcmTagSize    = 16
)

// EncryptGeneric encrypts plaintext using AES-256-GCM with a 12-byte IV.
func EncryptGeneric(plaintext string, key []byte) (EncryptedPayload, error) {
	if len(key) != aesKeySize {
		return EncryptedPayload{}, fmt.Errorf("key must be %d bytes, got %d", aesKeySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return EncryptedPayload{}, fmt.Errorf("creating cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedPayload{}, fmt.Errorf("creating GCM: %w", err)
	}

	iv := make([]byte, genericIVSize)
	if _, err := rand.Read(iv); err != nil {
		return EncryptedPayload{}, fmt.Errorf("generating IV: %w", err)
	}

	sealed := aesGCM.Seal(nil, iv, []byte(plaintext), nil)

	// GCM appends the auth tag to the ciphertext
	ciphertext := sealed[:len(sealed)-gcmTagSize]
	authTag := sealed[len(sealed)-gcmTagSize:]

	return EncryptedPayload{
		EncryptedData: hex.EncodeToString(ciphertext),
		IV:            hex.EncodeToString(iv),
		AuthTag:       hex.EncodeToString(authTag),
		Algorithm:     "aes-256-gcm",
	}, nil
}

// DecryptGeneric decrypts an EncryptedPayload using AES-256-GCM.
func DecryptGeneric(payload EncryptedPayload, key []byte) (string, error) {
	if len(key) != aesKeySize {
		return "", fmt.Errorf("key must be %d bytes, got %d", aesKeySize, len(key))
	}

	ciphertext, err := hex.DecodeString(payload.EncryptedData)
	if err != nil {
		return "", fmt.Errorf("decoding ciphertext: %w", err)
	}

	iv, err := hex.DecodeString(payload.IV)
	if err != nil {
		return "", fmt.Errorf("decoding IV: %w", err)
	}

	authTag, err := hex.DecodeString(payload.AuthTag)
	if err != nil {
		return "", fmt.Errorf("decoding auth tag: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	// Reassemble sealed = ciphertext + authTag
	sealed := append(ciphertext, authTag...)

	plaintext, err := aesGCM.Open(nil, iv, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	return string(plaintext), nil
}

// deriveCASKey derives a 32-byte key from secretKey using scrypt with a fixed salt.
func deriveCASKey(secretKey string) ([]byte, error) {
	return scrypt.Key([]byte(secretKey), []byte("GitAISalt"), 32768, 8, 1, aesKeySize)
}

// EncryptCAS encrypts plaintext for CAS storage using scrypt-derived key and 16-byte IV.
// Returns "iv_hex:authTag_hex:ciphertext_hex".
func EncryptCAS(plaintext string, secretKey string) (string, error) {
	key, err := deriveCASKey(secretKey)
	if err != nil {
		return "", fmt.Errorf("deriving key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCMWithNonceSize(block, casIVSize)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	iv := make([]byte, casIVSize)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("generating IV: %w", err)
	}

	sealed := aesGCM.Seal(nil, iv, []byte(plaintext), nil)

	ciphertext := sealed[:len(sealed)-gcmTagSize]
	authTag := sealed[len(sealed)-gcmTagSize:]

	return fmt.Sprintf("%s:%s:%s",
		hex.EncodeToString(iv),
		hex.EncodeToString(authTag),
		hex.EncodeToString(ciphertext),
	), nil
}

// DecryptCAS decrypts a CAS-encrypted string in "iv:authTag:ciphertext" hex format.
func DecryptCAS(encrypted string, secretKey string) (string, error) {
	parts := strings.SplitN(encrypted, ":", 3)
	if len(parts) != 3 {
		return "", errors.New("invalid CAS encrypted format: expected iv:authTag:ciphertext")
	}

	iv, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decoding IV: %w", err)
	}

	authTag, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decoding auth tag: %w", err)
	}

	ciphertext, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decoding ciphertext: %w", err)
	}

	key, err := deriveCASKey(secretKey)
	if err != nil {
		return "", fmt.Errorf("deriving key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCMWithNonceSize(block, casIVSize)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	sealed := append(ciphertext, authTag...)

	plaintext, err := aesGCM.Open(nil, iv, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	return string(plaintext), nil
}
