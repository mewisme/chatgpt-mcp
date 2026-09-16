package workspacestate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitpkg "go.mewis.me/chatgpt-mcp/internal/git"
	"go.mewis.me/chatgpt-mcp/internal/idgen"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

const (
	DirectoryName   = ".cgm"
	identityVersion = 1
	configVersion   = 1
)

type Identity struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type Config struct {
	Version   int      `json:"version"`
	AllowDirs []string `json:"allow_dirs,omitempty"`
	LegacyIDs []string `json:"legacy_ids,omitempty"`
}

type Store struct {
	WorkspaceRoot string
}

func New(root string) Store { return Store{WorkspaceRoot: filepath.Clean(root)} }
func DefaultConfig() Config { return Config{Version: configVersion} }

func (s Store) Root() string                 { return filepath.Join(s.WorkspaceRoot, DirectoryName) }
func (s Store) IdentityPath() string         { return filepath.Join(s.Root(), "workspace.json") }
func (s Store) ConfigPath() string           { return filepath.Join(s.Root(), "config.json") }
func (s Store) StateRoot() string            { return filepath.Join(s.Root(), "state") }
func (s Store) MemoryRoot() string           { return filepath.Join(s.Root(), "memory") }
func (s Store) CheckpointRoot() string       { return filepath.Join(s.Root(), "checkpoints") }
func (s Store) CacheRoot() string            { return filepath.Join(s.Root(), "cache") }
func (s Store) RuntimeRoot() string          { return filepath.Join(s.Root(), "runtime") }
func (s Store) RuntimeLockPath() string      { return filepath.Join(s.RuntimeRoot(), "lock") }
func (s Store) PluginsRoot() string          { return filepath.Join(s.Root(), "plugins") }
func (s Store) StatePath(name string) string { return filepath.Join(s.StateRoot(), name) }

func (s Store) EnsureIdentity(preferredID string) (Identity, bool, error) {
	identity, err := s.LoadIdentity()
	if err == nil {
		if preferredID != "" && identity.ID != preferredID {
			return Identity{}, false, fmt.Errorf("workspace identity mismatch: local %s, expected %s", identity.ID, preferredID)
		}
		return identity, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Identity{}, false, err
	}
	id := strings.TrimSpace(preferredID)
	if id == "" {
		id, err = idgen.New("ws", 8)
		if err != nil {
			return Identity{}, false, err
		}
	}
	identity = Identity{Version: identityVersion, ID: id, CreatedAt: time.Now().UTC()}
	if err := validateIdentity(identity); err != nil {
		return Identity{}, false, err
	}
	if err := writeJSONAtomic(s.IdentityPath(), identity); err != nil {
		return Identity{}, false, err
	}
	return identity, true, nil
}

func (s Store) LoadIdentity() (Identity, error) {
	data, err := os.ReadFile(s.IdentityPath())
	if err != nil {
		return Identity{}, err
	}
	var identity Identity
	if err := decodeStrict(data, &identity); err != nil {
		return Identity{}, fmt.Errorf("decode workspace identity: %w", err)
	}
	if err := validateIdentity(identity); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

func (s Store) LoadConfig() (Config, error) {
	data, err := os.ReadFile(s.ConfigPath())
	if err != nil {
		return Config{}, err
	}
	var config Config
	if err := decodeStrict(data, &config); err != nil {
		return Config{}, fmt.Errorf("decode workspace config: %w", err)
	}
	if config.Version != configVersion {
		return Config{}, fmt.Errorf("unsupported workspace config version: %d", config.Version)
	}
	return config, nil
}

func (s Store) SaveConfig(config Config) error {
	config.Version = configVersion
	return writeJSONAtomic(s.ConfigPath(), config)
}

func (s Store) EnsureGitExcluded(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	top, err := gitpkg.Run(ctx, s.WorkspaceRoot, "rev-parse", "--show-toplevel")
	if err != nil || top.ExitCode != 0 || strings.TrimSpace(top.Stdout) == "" {
		return nil
	}
	gitRoot := filepath.Clean(strings.TrimSpace(top.Stdout))
	relative, err := filepath.Rel(gitRoot, s.WorkspaceRoot)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}
	pattern := "/.cgm/"
	if relative != "." {
		pattern = "/" + strings.Trim(filepath.ToSlash(relative), "/") + "/.cgm/"
	}
	exclude, err := gitpkg.Run(ctx, s.WorkspaceRoot, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil || exclude.ExitCode != 0 || strings.TrimSpace(exclude.Stdout) == "" {
		exclude, err = gitpkg.Run(ctx, s.WorkspaceRoot, "rev-parse", "--git-path", "info/exclude")
		if err != nil || exclude.ExitCode != 0 || strings.TrimSpace(exclude.Stdout) == "" {
			return nil
		}
	}
	excludePath := strings.TrimSpace(exclude.Stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(s.WorkspaceRoot, excludePath)
	}
	data, readErr := os.ReadFile(excludePath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	updated := string(data)
	if updated != "" && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += pattern + "\n"
	if err := os.MkdirAll(filepath.Dir(excludePath), 0700); err != nil {
		return err
	}
	return state.WriteFileAtomic(excludePath, []byte(updated), 0600)
}

func (s Store) MigrateLegacyState(source string) error {
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("legacy workspace state is not a directory: %s", source)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		destination := filepath.Join(s.StateRoot(), entry.Name())
		switch {
		case entry.Name() == "MEMORY.md":
			destination = filepath.Join(s.MemoryRoot(), "MEMORY.md")
		case entry.Name() == "checkpoints":
			destination = s.CheckpointRoot()
		case strings.HasPrefix(entry.Name(), "shell."):
			destination = filepath.Join(s.StateRoot(), entry.Name())
		}
		if err := copyVerified(filepath.Join(source, entry.Name()), destination); err != nil {
			return fmt.Errorf("migrate legacy workspace state %s: %w", entry.Name(), err)
		}
	}
	if err := os.RemoveAll(source); err != nil {
		return err
	}
	parent := filepath.Dir(source)
	if filepath.Base(parent) != "workspaces" {
		return nil
	}
	entries, err = os.ReadDir(parent)
	if err != nil || len(entries) > 0 {
		return nil
	}
	return os.Remove(parent)
}

func validateIdentity(identity Identity) error {
	if identity.Version != identityVersion {
		return fmt.Errorf("unsupported workspace identity version: %d", identity.Version)
	}
	if !strings.HasPrefix(identity.ID, "ws_") || len(identity.ID) <= len("ws_") {
		return errors.New("workspace identity contains invalid id")
	}
	if identity.CreatedAt.IsZero() {
		return errors.New("workspace identity contains invalid created_at")
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(path, append(data, '\n'), 0600)
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func copyVerified(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to migrate symlink: %s", source)
	}
	if info.IsDir() {
		if existing, err := os.Stat(destination); err == nil && !existing.IsDir() {
			return fmt.Errorf("migration destination is not a directory: %s", destination)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(destination, 0700); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyVerified(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to migrate unsupported file type: %s", source)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(destination); err == nil {
		if string(existing) == string(data) {
			return nil
		}
		if filepath.Base(destination) == "index.json" {
			if merged, ok := mergeCheckpointIndex(existing, data); ok {
				return state.WriteFileAtomic(destination, merged, 0600)
			}
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return state.WriteFileAtomic(destination, data, 0600)
}

func mergeCheckpointIndex(dest, src []byte) ([]byte, bool) {
	type index struct {
		Version     int               `json:"version"`
		Checkpoints []json.RawMessage `json:"checkpoints"`
	}
	var local, leftover index
	if json.Unmarshal(dest, &local) != nil || json.Unmarshal(src, &leftover) != nil {
		return nil, false
	}
	if local.Version != 1 || leftover.Version != 1 || (local.Checkpoints == nil && leftover.Checkpoints == nil) {
		return nil, false
	}
	type entry struct {
		id, created string
		raw         json.RawMessage
	}
	parse := func(raw json.RawMessage) (entry, bool) {
		var meta struct {
			ID        string `json:"id"`
			CreatedAt string `json:"created_at"`
		}
		if json.Unmarshal(raw, &meta) != nil || meta.ID == "" {
			return entry{}, false
		}
		return entry{id: meta.ID, created: meta.CreatedAt, raw: raw}, true
	}
	seen := make(map[string]struct{}, len(local.Checkpoints)+len(leftover.Checkpoints))
	merged := make([]entry, 0, len(local.Checkpoints)+len(leftover.Checkpoints))
	add := func(raw json.RawMessage) {
		next, ok := parse(raw)
		if !ok {
			return
		}
		if _, dup := seen[next.id]; dup {
			return
		}
		seen[next.id] = struct{}{}
		merged = append(merged, next)
	}
	for _, raw := range local.Checkpoints {
		add(raw)
	}
	for _, raw := range leftover.Checkpoints {
		add(raw)
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].created < merged[j].created })
	out := index{Version: 1, Checkpoints: make([]json.RawMessage, 0, len(merged))}
	for _, next := range merged {
		out.Checkpoints = append(out.Checkpoints, next.raw)
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, false
	}
	return append(data, '\n'), true
}
