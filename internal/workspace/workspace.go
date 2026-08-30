package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

const storeVersion = 1

var ErrNotFound = errors.New("workspace not found")

type Workspace struct {
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	AllowDirs []string `json:"allow_dirs,omitempty"`
}

type storeFile struct {
	Version    int         `json:"version"`
	Workspaces []Workspace `json:"workspaces"`
}

type Manager struct {
	path            string
	mu              sync.RWMutex
	loaded          bool
	items           map[string]Workspace
	globalAllowDirs []string
}

func DefaultStorePath() string {
	return configformat.StructuredPath(configformat.RootPath(), "workspaces")
}

func NewManager(path string) *Manager {
	return &Manager{path: path, items: map[string]Workspace{}}
}

func NewManagerWithGlobalAllowDirs(path string, allowDirs []string) *Manager {
	manager := NewManager(path)
	manager.globalAllowDirs = normalizeRoots(allowDirs)
	return manager
}

func (m *Manager) SetGlobalAllowDirs(allowDirs []string) {
	m.mu.Lock()
	m.globalAllowDirs = normalizeRoots(allowDirs)
	m.mu.Unlock()
}

func (m *Manager) Register(path string) (Workspace, error) {
	if err := m.ensureLoaded(); err != nil {
		return Workspace{}, err
	}
	root, err := canonicalExistingDirectory(path)
	if err != nil {
		return Workspace{}, err
	}
	sum := sha256.Sum256([]byte(normalizeForID(root)))
	item := Workspace{ID: "ws_" + hex.EncodeToString(sum[:])[:16], Path: root, AllowDirs: []string{}}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.items[item.ID]; ok {
		item.AllowDirs = append([]string(nil), existing.AllowDirs...)
	}
	m.items[item.ID] = item
	if err := m.saveLocked(); err != nil {
		return Workspace{}, err
	}
	return item, nil
}

func (m *Manager) AddAllowDir(id, path string) (Workspace, error) {
	root, err := canonicalExistingDirectory(path)
	if err != nil {
		return Workspace{}, err
	}
	if err := m.ensureLoaded(); err != nil {
		return Workspace{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[id]
	if !ok {
		return Workspace{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	item.AllowDirs = normalizeRoots(append(item.AllowDirs, root))
	m.items[id] = item
	if err := m.saveLocked(); err != nil {
		return Workspace{}, err
	}
	return item, nil
}

func (m *Manager) RemoveAllowDir(id, path string) (Workspace, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Workspace{}, err
	}
	root := filepath.Clean(absolute)
	if canonical, err := filepath.EvalSymlinks(root); err == nil {
		root = filepath.Clean(canonical)
	}
	if err := m.ensureLoaded(); err != nil {
		return Workspace{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[id]
	if !ok {
		return Workspace{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	filtered := item.AllowDirs[:0]
	removed := false
	for _, value := range item.AllowDirs {
		if filepath.Clean(value) == root {
			removed = true
			continue
		}
		filtered = append(filtered, value)
	}
	if !removed {
		return Workspace{}, fmt.Errorf("workspace allowed directory is not configured: %s", root)
	}
	item.AllowDirs = normalizeRoots(filtered)
	m.items[id] = item
	if err := m.saveLocked(); err != nil {
		return Workspace{}, err
	}
	return item, nil
}

func (m *Manager) EffectiveRoots(id string) ([]string, error) {
	item, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	global := append([]string(nil), m.globalAllowDirs...)
	m.mu.RUnlock()
	roots := []string{item.Path}
	roots = append(roots, global...)
	roots = append(roots, item.AllowDirs...)
	return normalizeRoots(roots), nil
}

func (m *Manager) Get(id string) (Workspace, error) {
	if err := m.ensureLoaded(); err != nil {
		return Workspace{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.items[id]
	if !ok {
		return Workspace{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return item, nil
}

func (m *Manager) List() ([]Workspace, error) {
	if err := m.ensureLoaded(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := make([]Workspace, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items, nil
}

func (m *Manager) ResolveWorkingDirectory(id, input string) (Workspace, string, error) {
	item, err := m.Get(id)
	if err != nil {
		return Workspace{}, "", err
	}
	if strings.TrimSpace(input) == "" {
		input = item.Path
	}
	candidate := input
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(item.Path, candidate)
	}
	canonical, err := canonicalExistingDirectory(candidate)
	if err != nil {
		return Workspace{}, "", err
	}
	if !m.allowed(item.ID, canonical) {
		return Workspace{}, "", fmt.Errorf("working_directory escapes workspace: %s", canonical)
	}
	return item, canonical, nil
}

func (m *Manager) ResolvePath(id, workingDirectory, input string, mustExist bool) (string, error) {
	item, cwd, err := m.ResolveWorkingDirectory(id, workingDirectory)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(input) == "" {
		return "", errors.New("path is required")
	}
	candidate := input
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(cwd, candidate)
	}
	canonical, err := canonicalForContainment(candidate, mustExist)
	if err != nil {
		return "", err
	}
	if !m.allowed(item.ID, canonical) {
		return "", fmt.Errorf("path escapes workspace: %s", canonical)
	}
	return canonical, nil
}

func (m *Manager) ensureLoaded() error {
	m.mu.RLock()
	if m.loaded {
		m.mu.RUnlock()
		return nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loaded {
		return nil
	}

	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		m.loaded = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("read workspace registry: %w", err)
	}

	var stored storeFile
	if err := configformat.UnmarshalPath(m.path, data, &stored); err != nil {
		return fmt.Errorf("decode workspace registry: %w", err)
	}
	if stored.Version != storeVersion {
		return fmt.Errorf("unsupported workspace registry version: %d", stored.Version)
	}
	for _, item := range stored.Workspaces {
		if item.ID == "" || item.Path == "" {
			return errors.New("workspace registry contains invalid entry")
		}
		item.AllowDirs = normalizeRoots(item.AllowDirs)
		m.items[item.ID] = item
	}
	m.loaded = true
	return nil
}

func (m *Manager) allowed(id, candidate string) bool {
	roots, err := m.EffectiveRoots(id)
	if err != nil {
		return false
	}
	for _, root := range roots {
		if within(root, candidate) {
			return true
		}
	}
	return false
}

func normalizeRoots(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			continue
		}
		root := filepath.Clean(absolute)
		if canonical, err := filepath.EvalSymlinks(root); err == nil {
			root = filepath.Clean(canonical)
		}
		result = appendUniqueRoot(result, root)
	}
	sort.Strings(result)
	return result
}

func appendUniqueRoot(values []string, root string) []string {
	for _, value := range values {
		if filepath.Clean(value) == filepath.Clean(root) {
			return values
		}
	}
	return append(values, root)
}

func (m *Manager) saveLocked() error {
	items := make([]Workspace, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	data, err := configformat.MarshalPath(m.path, storeFile{Version: storeVersion, Workspaces: items})
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(m.path, data, 0600)
}

func canonicalExistingDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", absolute)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(canonical), nil
}

func canonicalForContainment(path string, mustExist bool) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if mustExist {
		canonical, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return "", err
		}
		return filepath.Clean(canonical), nil
	}
	if canonical, err := filepath.EvalSymlinks(absolute); err == nil {
		return filepath.Clean(canonical), nil
	}

	current := absolute
	var suffix []string
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("cannot resolve path parent: %s", absolute)
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
	canonicalParent, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	for i := len(suffix) - 1; i >= 0; i-- {
		canonicalParent = filepath.Join(canonicalParent, suffix[i])
	}
	return filepath.Clean(canonicalParent), nil
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}

func normalizeForID(path string) string {
	clean := filepath.Clean(path)
	if filepath.Separator == '\\' {
		return strings.ToLower(clean)
	}
	return clean
}
