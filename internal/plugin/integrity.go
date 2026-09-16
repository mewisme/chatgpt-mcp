package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const payloadIntegrityFile = "payload.sha256"

func writePayloadIntegrity(root, payload string) error {
	digest, err := payloadTreeSHA256(payload)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, payloadIntegrityFile), []byte(digest+"\n"), 0600)
}

func verifyPackagedPayload(installed InstalledPlugin) error {
	if strings.TrimSpace(installed.Payload) == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(installed.Root, payloadIntegrityFile))
	if err != nil {
		return fmt.Errorf("read packaged payload integrity metadata: %w", err)
	}
	expected := strings.TrimSpace(string(data))
	if !validSHA256(expected) {
		return errors.New("packaged payload integrity metadata is invalid")
	}
	actual, err := payloadTreeSHA256(installed.Payload)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("packaged payload integrity verification failed: expected %s, got %s", expected, actual)
	}
	return nil
}

func payloadTreeSHA256(root string) (string, error) {
	root = filepath.Clean(root)
	entries := []string{}
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root {
			if !entry.IsDir() {
				return errors.New("plugin payload root must be a directory")
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("plugin payload symlink is not allowed: %s", current)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("plugin payload special file is not allowed: %s", current)
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	hash := sha256.New()
	for _, relative := range entries {
		fullPath := filepath.Join(root, filepath.FromSlash(relative))
		info, err := os.Stat(fullPath)
		if err != nil {
			return "", err
		}
		if _, err := io.WriteString(hash, fmt.Sprintf("file\x00%s\x00%04o\x00", relative, info.Mode().Perm())); err != nil {
			return "", err
		}
		file, err := os.Open(fullPath)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if _, err := hash.Write([]byte{0}); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
