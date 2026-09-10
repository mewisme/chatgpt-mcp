package secretstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.mewis.me/chatgpt-mcp/internal/state"
)

type fileBackend struct {
	root string
	mu   sync.Mutex
	key  []byte
}

func newFileBackend(root string) Backend {
	return &fileBackend{root: filepath.Join(root, "state", "secrets")}
}

func (b *fileBackend) Set(service, account, value string) error {
	path, err := b.path(service, account)
	if err != nil {
		return err
	}
	sealed, err := b.seal([]byte(value))
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(path, sealed, 0600)
}

func (b *fileBackend) Get(service, account string) (string, error) {
	path, err := b.path(service, account)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("secret path is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if isEncryptedBlob(data) {
		plaintext, err := b.open(data)
		if err != nil {
			return "", err
		}
		return string(plaintext), nil
	}
	value := string(data)
	// Legacy plaintext: keep readable and rewrite encrypted when possible.
	_ = b.Set(service, account, value)
	return value, nil
}

func (b *fileBackend) Delete(service, account string) error {
	path, err := b.path(service, account)
	if err != nil {
		return err
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	} else {
		return err
	}
}

func (b *fileBackend) MigratePlaintext() (int, error) {
	entries, err := os.ReadDir(b.root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	migrated := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".secret") {
			continue
		}
		path := filepath.Join(b.root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return migrated, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return migrated, err
		}
		if isEncryptedBlob(data) {
			continue
		}
		sealed, err := b.seal(data)
		if err != nil {
			return migrated, err
		}
		if err := state.WriteFileAtomic(path, sealed, 0600); err != nil {
			return migrated, err
		}
		migrated++
	}
	return migrated, nil
}

func (b *fileBackend) path(service, account string) (string, error) {
	service = strings.TrimSpace(service)
	account = strings.TrimSpace(account)
	if service == "" {
		return "", errors.New("secret service is required")
	}
	if account == "" {
		return "", errors.New("secret account is required")
	}
	sum := sha256.Sum256([]byte(service + "\x00" + account))
	return filepath.Join(b.root, hex.EncodeToString(sum[:])+".secret"), nil
}
