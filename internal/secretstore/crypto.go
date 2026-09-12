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
	"time"
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
	root, err := b.openConfigRoot(true)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir := filepath.Join("state", "secrets")
	if err := root.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, masterKeyName)
	data, err := readMasterKey(root, path)
	if err == nil {
		b.key = data
		return b.key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, masterKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		data, err := waitForMasterKey(root, path)
		if err != nil {
			return nil, err
		}
		b.key = data
		return b.key, nil
	}
	if err != nil {
		return nil, fmt.Errorf("create master key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write master key: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("sync master key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close master key: %w", err)
	}
	b.key = key
	return b.key, nil
}

func readMasterKey(root *os.Root, path string) ([]byte, error) {
	data, err := readRootedRegularFile(root, path)
	if err != nil {
		return nil, err
	}
	if len(data) != masterKeySize {
		return nil, fmt.Errorf("master key %s has invalid length %d", path, len(data))
	}
	return append([]byte(nil), data...), nil
}

func waitForMasterKey(root *os.Root, path string) ([]byte, error) {
	var err error
	for range 50 {
		var data []byte
		data, err = readMasterKey(root, path)
		if err == nil {
			return data, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		time.Sleep(time.Millisecond)
	}
	return nil, err
}
