package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	statepkg "go.mewis.me/chatgpt-mcp/internal/state"
)

const environmentVersion = 1

type EnvironmentSnapshot struct {
	Version int               `json:"version"`
	Values  map[string]string `json:"values"`
}

func CaptureEnvironment(account Account, extraPath []string) EnvironmentSnapshot {
	values := map[string]string{}
	for _, key := range []string{"SHELL", "COMSPEC", "PATHEXT", "LANG", "TERM", "TMPDIR", "TEMP", "TMP", "DBUS_SESSION_BUS_ADDRESS", "XDG_RUNTIME_DIR"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			values[key] = value
		}
	}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(key, "LC_") && value != "" {
			values[key] = value
		}
	}
	path := mergeExecutablePath(extraPath, filepath.SplitList(os.Getenv("PATH")))
	if len(path) > 0 {
		values["PATH"] = strings.Join(path, string(os.PathListSeparator))
	}
	if runtime.GOOS == "windows" {
		values["USERPROFILE"] = account.HomeDir
		values["USERNAME"] = account.Username
	} else {
		values["HOME"] = account.HomeDir
		values["USER"] = account.Username
		values["LOGNAME"] = account.Username
	}
	return EnvironmentSnapshot{Version: environmentVersion, Values: values}
}

func SaveEnvironment(configRoot string, snapshot EnvironmentSnapshot) (string, error) {
	if snapshot.Version != environmentVersion || snapshot.Values == nil {
		return "", errors.New("invalid managed environment snapshot")
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	path := EnvironmentPath(configRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	if err := statepkg.WriteFileAtomic(path, data, 0600); err != nil {
		return "", err
	}
	return environmentHash(data), nil
}

func LoadEnvironment(configRoot, expectedHash string) (EnvironmentSnapshot, error) {
	data, err := os.ReadFile(EnvironmentPath(configRoot))
	if err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("read managed environment: %w", err)
	}
	if expectedHash != "" && environmentHash(data) != expectedHash {
		return EnvironmentSnapshot{}, errors.New("managed environment snapshot does not match service definition; run cgm up again")
	}
	var snapshot EnvironmentSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("decode managed environment: %w", err)
	}
	if snapshot.Version != environmentVersion || snapshot.Values == nil {
		return EnvironmentSnapshot{}, errors.New("unsupported managed environment snapshot")
	}
	return snapshot, nil
}

func ApplyEnvironment(snapshot EnvironmentSnapshot) error {
	keys := make([]string, 0, len(snapshot.Values))
	for key := range snapshot.Values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := os.Setenv(key, snapshot.Values[key]); err != nil {
			return fmt.Errorf("set managed environment %s: %w", key, err)
		}
	}
	return nil
}

func EnvironmentPath(configRoot string) string {
	return filepath.Join(configRoot, "runtime", "environment.json")
}

func environmentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func mergeExecutablePath(groups ...[]string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, group := range groups {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			value = filepath.Clean(value)
			key := value
			if runtime.GOOS == "windows" {
				key = strings.ToLower(key)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, value)
		}
	}
	return result
}
