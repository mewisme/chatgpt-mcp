package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/instance"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

const storeVersion = 3

var ErrNotFound = errors.New("workspace not found")

type Workspace struct {
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	AllowDirs []string `json:"allow_dirs,omitempty"`
	LegacyIDs []string `json:"legacy_ids,omitempty"`
}

type storeFile struct {
	Version    int         `json:"version"`
	Workspaces []Workspace `json:"workspaces"`
}

type Manager struct {
	path            string
	protectedRoot   string
	instanceStore   *instance.Store
	identityOnce    sync.Once
	identity        instance.Identity
	identityErr     error
	mu              sync.RWMutex
	loaded          bool
	items           map[string]Workspace
	aliases         map[string]string
	globalAllowDirs []string
}

func DefaultStorePath() string {
	return configformat.StructuredPath(configformat.RootPath(), "workspaces")
}

func NewManager(path string) *Manager {
	protectedRoot := ""
	storeRoot := canonicalRoot(filepath.Dir(path))
	configRoot := canonicalRoot(configformat.RootPath())
	if storeRoot != "" && configRoot != "" && storeRoot == configRoot {
		protectedRoot = configRoot
	}
	return &Manager{path: path, protectedRoot: protectedRoot, instanceStore: instance.NewStore(filepath.Dir(path)), items: map[string]Workspace{}, aliases: map[string]string{}}
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
	if m.protected(root) {
		return Workspace{}, fmt.Errorf("workspace root is inside protected control-plane state: %s", root)
	}
	item := Workspace{ID: workspaceID(root), Path: root, AllowDirs: []string{}}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.items[item.ID]; ok {
		item.AllowDirs = append([]string(nil), existing.AllowDirs...)
		item.LegacyIDs = append([]string(nil), existing.LegacyIDs...)
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
	if m.protected(root) {
		return Workspace{}, fmt.Errorf("allowed directory is inside protected control-plane state: %s", root)
	}
	if err := m.ensureLoaded(); err != nil {
		return Workspace{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	canonical := m.canonicalIDLocked(id)
	item, ok := m.items[canonical]
	if !ok {
		return Workspace{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	item.AllowDirs = normalizeRoots(append(item.AllowDirs, root))
	m.items[canonical] = item
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
	canonical := m.canonicalIDLocked(id)
	item, ok := m.items[canonical]
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
	m.items[canonical] = item
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
	canonical := m.canonicalIDLocked(id)
	item, ok := m.items[canonical]
	if !ok {
		return Workspace{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return item, nil
}

func (m *Manager) CanonicalID(id string) (string, error) {
	if err := m.ensureLoaded(); err != nil {
		return "", err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	canonical := m.canonicalIDLocked(id)
	if _, ok := m.items[canonical]; !ok {
		return "", fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return canonical, nil
}

func (m *Manager) Instance() (instance.Identity, error) {
	m.identityOnce.Do(func() { m.identity, m.identityErr = m.instanceStore.LoadOrCreate() })
	return m.identity, m.identityErr
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

func (m *Manager) AdvertisedIDs() ([]string, error) {
	items, err := m.List()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = appendUniqueString(ids, item.ID)
		for _, legacyID := range item.LegacyIDs {
			ids = appendUniqueString(ids, legacyID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (m *Manager) ResolveDirectory(id, input string) (Workspace, string, error) {
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
		return Workspace{}, "", fmt.Errorf("directory escapes workspace: %s", canonical)
	}
	return item, canonical, nil
}

func (m *Manager) ResolvePath(id, baseDirectory, input string, mustExist bool) (string, error) {
	item, cwd, err := m.ResolveDirectory(id, baseDirectory)
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
	if stored.Version < 1 || stored.Version > storeVersion {
		return fmt.Errorf("unsupported workspace registry version: %d", stored.Version)
	}
	migrated := stored.Version != storeVersion
	for _, item := range stored.Workspaces {
		if item.ID == "" || item.Path == "" {
			return errors.New("workspace registry contains invalid entry")
		}
		canonicalID := workspaceID(item.Path)
		if item.ID != canonicalID {
			previousID := item.ID
			item.ID = canonicalID
			item.LegacyIDs = appendUniqueString(item.LegacyIDs, previousID)
			if err := m.migrateWorkspaceState(previousID, canonicalID); err != nil {
				return err
			}
		}
		item.AllowDirs = normalizeRoots(item.AllowDirs)
		item.LegacyIDs = normalizeIDs(item.LegacyIDs, item.ID)
		if existing, ok := m.items[item.ID]; ok && filepath.Clean(existing.Path) != filepath.Clean(item.Path) {
			return fmt.Errorf("workspace registry id collision: %s", item.ID)
		}
		m.items[item.ID] = item
		for _, alias := range item.LegacyIDs {
			if err := m.registerAliasLocked(alias, item.ID); err != nil {
				return err
			}
		}
	}
	m.loaded = true
	if migrated {
		if err := m.saveLocked(); err != nil {
			m.loaded = false
			return fmt.Errorf("persist migrated workspace registry: %w", err)
		}
	}
	return nil
}

func (m *Manager) canonicalIDLocked(id string) string {
	if _, ok := m.items[id]; ok {
		return id
	}
	if canonical := m.aliases[id]; canonical != "" {
		return canonical
	}
	return id
}

func (m *Manager) registerAliasLocked(alias, canonical string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" || alias == canonical {
		return nil
	}
	if existing, ok := m.items[alias]; ok && existing.ID != canonical {
		return fmt.Errorf("workspace legacy id collides with workspace id: %s", alias)
	}
	if existing := m.aliases[alias]; existing != "" && existing != canonical {
		return fmt.Errorf("workspace legacy id collision: %s", alias)
	}
	m.aliases[alias] = canonical
	return nil
}

func (m *Manager) migrateWorkspaceState(legacyID, canonicalID string) error {
	if legacyID == canonicalID {
		return nil
	}
	root := filepath.Join(filepath.Dir(m.path), "workspaces")
	legacyPath := filepath.Join(root, legacyID)
	canonicalPath := filepath.Join(root, canonicalID)
	_, legacyErr := os.Stat(legacyPath)
	_, canonicalErr := os.Stat(canonicalPath)
	if errors.Is(legacyErr, os.ErrNotExist) {
		return nil
	}
	if legacyErr != nil {
		return fmt.Errorf("inspect legacy workspace state: %w", legacyErr)
	}
	if canonicalErr == nil {
		return fmt.Errorf("cannot migrate workspace state: both %s and %s exist", legacyPath, canonicalPath)
	}
	if !errors.Is(canonicalErr, os.ErrNotExist) {
		return fmt.Errorf("inspect migrated workspace state: %w", canonicalErr)
	}
	if err := rewriteWorkspaceStateIDs(legacyPath, legacyID, canonicalID); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	if err := os.Rename(legacyPath, canonicalPath); err != nil {
		return fmt.Errorf("migrate workspace state %s -> %s: %w", legacyID, canonicalID, err)
	}
	return nil
}

func rewriteWorkspaceStateIDs(root, legacyID, canonicalID string) error {
	paths := []string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return err
	}
	for _, path := range paths {
		format, err := configformat.Detect(path)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		decoded, err := configformat.DecodeGeneric(format, data)
		if err != nil {
			return fmt.Errorf("decode workspace state %s: %w", path, err)
		}
		object, ok := decoded.(map[string]any)
		if !ok {
			continue
		}
		value, _ := object["workspace_id"].(string)
		if value == "" || value == canonicalID {
			continue
		}
		if value != legacyID {
			return fmt.Errorf("workspace state %s belongs to unexpected workspace %s", path, value)
		}
		object["workspace_id"] = canonicalID
		encoded, err := configformat.EncodeGeneric(format, object)
		if err != nil {
			return fmt.Errorf("encode workspace state %s: %w", path, err)
		}
		if err := state.WriteFileAtomic(path, encoded, 0600); err != nil {
			return fmt.Errorf("rewrite workspace state %s: %w", path, err)
		}
	}
	return nil
}

func (m *Manager) allowed(id, candidate string) bool {
	if m.protected(candidate) {
		return false
	}
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

func (m *Manager) protected(candidate string) bool {
	return m.protectedRoot != "" && within(m.protectedRoot, candidate)
}

func canonicalRoot(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	root := filepath.Clean(absolute)
	if canonical, err := filepath.EvalSymlinks(root); err == nil {
		root = filepath.Clean(canonical)
	}
	return root
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

func normalizeIDs(values []string, canonical string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || value == canonical {
			continue
		}
		result = appendUniqueString(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	sort.Strings(result)
	return result
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
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

func workspaceID(path string) string {
	sum := sha256.Sum256([]byte(normalizeForID(path)))
	return "ws_" + hex.EncodeToString(sum[:])[:16]
}

func IDForPath(path string) string { return workspaceID(path) }

func instanceScopedWorkspaceID(instanceID, path string) string {
	sum := sha256.Sum256([]byte(instanceID + "\x00" + normalizeForID(path)))
	return "ws_" + hex.EncodeToString(sum[:])[:16]
}
