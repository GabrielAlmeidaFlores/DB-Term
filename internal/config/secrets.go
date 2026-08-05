package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/scrypt"
)

// SecretsStore is the on-disk format: a map of connection name → encrypted blob.
// Each blob is base64(nonce + AES-256-GCM ciphertext).
// The file is stored at ~/.config/dbterm/.secrets with permissions 0600.
type SecretsStore map[string]string

// secretsPath returns the full path to the .secrets file.
func secretsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".secrets"), nil
}

// LoadSecrets reads ~/.config/dbterm/.secrets.
// Returns an empty store (not an error) if the file does not exist yet.
func LoadSecrets() (SecretsStore, error) {
	path, err := secretsPath()
	if err != nil {
		return nil, fmt.Errorf("secrets: resolving path: %w", err)
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(SecretsStore), nil
	}
	if err != nil {
		return nil, fmt.Errorf("secrets: reading file: %w", err)
	}

	store := make(SecretsStore)
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("secrets: parsing JSON: %w", err)
	}
	return store, nil
}

// Save writes the SecretsStore to disk with 0600 permissions.
func (s SecretsStore) Save() error {
	dir, err := ConfigDir()
	if err != nil {
		return fmt.Errorf("secrets: resolving dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("secrets: creating directory: %w", err)
	}

	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("secrets: marshaling JSON: %w", err)
	}

	path := filepath.Join(dir, ".secrets")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("secrets: writing file: %w", err)
	}
	return nil
}

// SetPassword encrypts the given password for connName and stores it in the map.
// Call Save() afterwards to persist to disk.
func (s SecretsStore) SetPassword(connName, password string) error {
	key, err := machineKey()
	if err != nil {
		return fmt.Errorf("secrets: deriving key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("secrets: creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("secrets: creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("secrets: generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(password), nil)
	s[connName] = base64.StdEncoding.EncodeToString(ciphertext)
	return nil
}

// GetPassword decrypts and returns the password for connName.
// Returns an error if the connection has no stored password.
func (s SecretsStore) GetPassword(connName string) (string, error) {
	blob, ok := s[connName]
	if !ok {
		return "", fmt.Errorf("secrets: no password stored for %q", connName)
	}

	data, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return "", fmt.Errorf("secrets: decoding base64 for %q: %w", connName, err)
	}

	key, err := machineKey()
	if err != nil {
		return "", fmt.Errorf("secrets: deriving key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("secrets: creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("secrets: creating GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("secrets: ciphertext too short for %q", connName)
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("secrets: decryption failed for %q: %w", connName, err)
	}

	return string(plaintext), nil
}

// DeletePassword removes the stored password for connName.
// No-op if the connection has no stored password.
// Call Save() afterwards to persist the deletion.
func (s SecretsStore) DeletePassword(connName string) {
	delete(s, connName)
}

// machineKey derives a 32-byte AES key from a machine-specific identifier.
// The key is deterministic for a given hostname + OS user, making secrets
// tied to the local machine without storing the key anywhere.
func machineKey() ([]byte, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("getting hostname: %w", err)
	}

	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME") // Windows fallback
	}

	// Fixed salt derived from the machine identity.
	// Not secret — its purpose is domain separation, not entropy.
	rawSalt := sha256.Sum256([]byte("dbterm-v1:" + hostname + ":" + user))
	salt := rawSalt[:]

	// scrypt parameters: N=32768, r=8, p=1 → ~32ms on modern hardware.
	password := []byte(hostname + ":" + user)
	key, err := scrypt.Key(password, salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, fmt.Errorf("deriving scrypt key: %w", err)
	}

	return key, nil
}
