package update

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var ErrChecksumMismatch = errors.New("release checksum mismatch")

func VerifyChecksum(archivePath, checksumPath, assetName string) error {
	expected, err := expectedChecksum(checksumPath, assetName)
	if err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("%w for %s", ErrChecksumMismatch, assetName)
	}
	return nil
}

func expectedChecksum(path, assetName string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != assetName {
			continue
		}
		hash := fields[0]
		if len(hash) != sha256.Size*2 {
			return "", fmt.Errorf("invalid checksum for %s", assetName)
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return "", fmt.Errorf("invalid checksum for %s: %w", assetName, err)
		}
		return strings.ToLower(hash), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("checksum missing for %s", assetName)
}
