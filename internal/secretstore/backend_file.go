package secretstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/state"
)

type fileBackend struct {
	root string
}

func newFileBackend(root string) Backend {
	return fileBackend{root: filepath.Join(root, "state", "secrets")}
}

func (b fileBackend) Set(service, account, value string) error {
	path, err := b.path(service, account)
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(path, []byte(value), 0600)
}

func (b fileBackend) Get(service, account string) (string, error) {
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
	return string(data), nil
}

func (b fileBackend) Delete(service, account string) error {
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

func (b fileBackend) path(service, account string) (string, error) {
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
