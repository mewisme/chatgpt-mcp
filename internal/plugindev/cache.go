package plugindev

import (
	"os"
	"path/filepath"
	"strings"
)

const cacheMarker = "fingerprint"

func (ctx Context) SkipCache() bool {
	return ctx.Mode == ModeRebuild
}

func StageDir(parent string) (string, error) {
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	return os.MkdirTemp(parent, "stage-")
}

func Cached(dir, fingerprint string) bool {
	if strings.TrimSpace(fingerprint) == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, cacheMarker))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == fingerprint
}

func PromoteDir(temp, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	_ = os.RemoveAll(dest)
	return os.Rename(temp, dest)
}

func WriteCacheMarker(dir, fingerprint string) error {
	if fingerprint == "" || strings.Contains(fingerprint, string(filepath.Separator)) || strings.Contains(fingerprint, "..") {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, cacheMarker), []byte(fingerprint+"\n"), 0o600)
}
