package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

var (
	devMasterKey   []byte
	devMasterKeyMu sync.Mutex
	devCASKey      string
	devCASKeyMu    sync.Mutex
)

// ResolveEncryptionMasterKey resolves the 32-byte encryption master key.
// If envValue is set, it is parsed as a 64-character hex string.
// In production, the key must be provided. In dev, a random key is generated and cached.
func ResolveEncryptionMasterKey(envValue string, isProduction bool) ([]byte, error) {
	if envValue != "" {
		if len(envValue) != 64 {
			return nil, fmt.Errorf("encryption master key must be 64 hex characters, got %d", len(envValue))
		}
		key, err := hex.DecodeString(envValue)
		if err != nil {
			return nil, fmt.Errorf("invalid hex in encryption master key: %w", err)
		}
		return key, nil
	}

	if isProduction {
		return nil, errors.New("encryption master key is required in production")
	}

	devMasterKeyMu.Lock()
	defer devMasterKeyMu.Unlock()

	if devMasterKey != nil {
		return devMasterKey, nil
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating dev master key: %w", err)
	}
	devMasterKey = key
	return devMasterKey, nil
}

// ResolveCASEncryptionKey resolves the CAS encryption key string.
// If envValue is set, it is returned as-is.
// In production, the key must be provided. In dev, a random hex string is generated and cached.
func ResolveCASEncryptionKey(envValue string, isProduction bool) (string, error) {
	if envValue != "" {
		return envValue, nil
	}

	if isProduction {
		return "", errors.New("CAS encryption key is required in production")
	}

	devCASKeyMu.Lock()
	defer devCASKeyMu.Unlock()

	if devCASKey != "" {
		return devCASKey, nil
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating dev CAS key: %w", err)
	}
	devCASKey = hex.EncodeToString(b)
	return devCASKey, nil
}
