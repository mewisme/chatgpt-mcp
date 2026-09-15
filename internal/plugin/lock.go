package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/state"
)

const LockSchema = 1

var ErrLockCorrupt = errors.New("plugin lock is corrupt")

type LockFile struct {
	Schema  int                     `json:"schema"`
	Plugins map[PluginID]LockPlugin `json:"plugins"`
}

type LockPlugin struct {
	Registry       string  `json:"registry"`
	Publisher      string  `json:"publisher"`
	Version        Version `json:"version"`
	ManifestDigest string  `json:"manifest_digest"`
	ArtifactDigest string  `json:"artifact_digest"`
	Enabled        bool    `json:"enabled"`
}

func NewLockFile() LockFile { return LockFile{Schema: LockSchema, Plugins: map[PluginID]LockPlugin{}} }

func LoadLock(path string) (LockFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewLockFile(), nil
	}
	if err != nil {
		return LockFile{}, err
	}
	var lock LockFile
	if err := decodeStrictJSON(data, &lock); err != nil {
		return LockFile{}, fmt.Errorf("%w: decode plugin lock: %v", ErrLockCorrupt, err)
	}
	if err := lock.Validate(); err != nil {
		return LockFile{}, fmt.Errorf("%w: %v", ErrLockCorrupt, err)
	}
	return lock, nil
}

func WriteLock(path string, lock LockFile) error {
	if err := lock.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return state.WriteFileAtomic(path, data, 0600)
}

func (lock LockFile) Validate() error {
	if lock.Schema != LockSchema {
		return fmt.Errorf("unsupported plugin lock schema: %d", lock.Schema)
	}
	if lock.Plugins == nil {
		return errors.New("plugin lock plugins map is required")
	}
	ids := make([]string, 0, len(lock.Plugins))
	for id := range lock.Plugins {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, idValue := range ids {
		id := PluginID(idValue)
		entry := lock.Plugins[id]
		if !validCanonicalName(idValue) {
			return fmt.Errorf("invalid locked plugin id: %q", id)
		}
		if !validCanonicalName(entry.Registry) {
			return fmt.Errorf("invalid registry for plugin %s: %q", id, entry.Registry)
		}
		if !validCanonicalName(entry.Publisher) {
			return fmt.Errorf("invalid publisher for plugin %s: %q", id, entry.Publisher)
		}
		if err := validateVersion(string(entry.Version)); err != nil {
			return fmt.Errorf("invalid locked version for plugin %s: %q", id, entry.Version)
		}
		if !validPrefixedSHA256(entry.ManifestDigest) || !validPrefixedSHA256(entry.ArtifactDigest) {
			return fmt.Errorf("invalid digest for plugin %s", id)
		}
	}
	return nil
}

func validPrefixedSHA256(value string) bool {
	return strings.HasPrefix(value, "sha256:") && validSHA256(strings.TrimPrefix(value, "sha256:"))
}
