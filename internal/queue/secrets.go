package queue

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"uplarr/internal/logger"
)

const secretKeyFile = ".secret_key"

// loadSecretKey returns the AES-256 key used to encrypt credentials in the
// queue state file. UPLARR_SECRET_KEY takes precedence so the key can live
// outside the config volume (e.g. a Docker secret); otherwise a random key is
// generated once and kept next to the state file.
func loadSecretKey(configDir string) ([]byte, error) {
	if v := strings.TrimSpace(os.Getenv("UPLARR_SECRET_KEY")); v != "" {
		sum := sha256.Sum256([]byte(v))
		return sum[:], nil
	}

	root, err := OsOpenRoot(configDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	data, err := root.ReadFile(secretKeyFile)
	if err == nil {
		key, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if decErr != nil || len(key) != 32 {
			return nil, fmt.Errorf("invalid %s", secretKeyFile)
		}
		return key, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := root.WriteFile(secretKeyFile, []byte(base64.StdEncoding.EncodeToString(key)), 0600); err != nil {
		return nil, err
	}
	logger.Info(fmt.Sprintf("Generated queue encryption key in %s", secretKeyFile))
	return key, nil
}

func encryptSecret(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptSecret(key []byte, encoded string) (string, error) {
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
