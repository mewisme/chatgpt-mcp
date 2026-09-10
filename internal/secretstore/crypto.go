package secretstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/state"
)

const (
	encryptedPrefix = "cgmsecret1:"
	masterKeyName   = ".master.key"
	masterKeySize   = 32
)

func isEncryptedBlob(data []byte) bool {
	return strings.HasPrefix(string(data), encryptedPrefix)
}

func (b *fileBackend) seal(plaintext []byte) ([]byte, error) {
	key, err := b.masterKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	encoded := encryptedPrefix + base64.StdEncoding.EncodeToString(sealed)
	return []byte(encoded), nil
}

func (b *fileBackend) open(data []byte) ([]byte, error) {
	raw := string(data)
	if !strings.HasPrefix(raw, encryptedPrefix) {
		return nil, errors.New("secret blob is not encrypted")
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, encryptedPrefix))
	if err != nil {
		return nil, fmt.Errorf("decode encrypted secret: %w", err)
	}
	key, err := b.masterKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize() {
		return nil, errors.New("encrypted secret is truncated")
	}
	nonce, ciphertext := payload[:gcm.NonceSize()], payload[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	return plaintext, nil
}

func (b *fileBackend) masterKey() ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.key) == masterKeySize {
		return b.key, nil
	}
	path := filepath.Join(b.root, masterKeyName)
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) != masterKeySize {
			return nil, fmt.Errorf("master key %s has invalid length %d", path, len(data))
		}
		b.key = append([]byte(nil), data...)
		return b.key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, masterKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := state.WriteFileAtomic(path, key, 0600); err != nil {
		return nil, fmt.Errorf("create master key: %w", err)
	}
	b.key = key
	return b.key, nil
}
