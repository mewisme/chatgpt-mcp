package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/idgen"
	"go.mewis.me/chatgpt-mcp/internal/instance"
	"go.mewis.me/chatgpt-mcp/internal/oslock"
	"go.mewis.me/chatgpt-mcp/internal/state"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
)

const storeVersion = 5

var (
	ErrNotFound    = errors.New("workspace not found")
	ErrUnavailable = errors.New("workspace unavailable")
)

type Workspace struct {
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	AllowDirs []string `json:"allow_dirs,omitempty"`
	LegacyIDs []string `json:"legacy_ids,omitempty"`
	Error     string   `json:"error,omitempty"`
}

func (item Workspace) Available() bool {
	return strings.TrimSpace(item.Error) == ""
}

func (item Workspace) unavailableError() error {
	if item.Available() {
		return nil
	}
	if strings.TrimSpace(item.ID) == "" {
		return fmt.Errorf("%w: %s", ErrUnavailable, item.Error)
	}
	return fmt.Errorf("%w: %s: %s", ErrUnavailable, item.ID, item.Error)
}

type storedWorkspace struct {
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	AllowDirs []string `json:"allow_dirs,omitempty"`
	LegacyIDs []string `json:"legacy_ids,omitempty"`
}

// WorkspaceContainer is an orchestration scope only. It groups concrete
// workspaces but never owns a filesystem root, shell cwd, memory, or rules.
type WorkspaceContainer struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	WorkspaceIDs []string `json:"workspace_ids,omitempty"`
}

type storeFile struct {
	Version    int                  `json:"version"`
	Workspaces []storedWorkspace    `json:"workspaces"`
	Containers []WorkspaceContainer `json:"containers,omitempty"`
}

type Manager struct {
	path            string
	trace           tracepkg.Observer
	protectedRoot   string
	instanceStore   *instance.Store
	identityOnce    sync.Once
	identity        instance.Identity
	identityErr     error
	runtimeMu       sync.Mutex
	mu              sync.RWMutex
	loaded          bool
	items           map[string]Workspace
	containers      map[string]WorkspaceContainer
	aliases         map[string]string
	globalAllowDirs []string
	shellPath       []string
	runtime         *runtimeState
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
	return &Manager{path: path, protectedRoot: protectedRoot, instanceStore: instance.NewStore(filepath.Dir(path)), items: map[string]Workspace{}, containers: map[string]WorkspaceContainer{}, aliases: map[string]string{}, runtime: newRuntimeState()}
}

func (m *Manager) SetTraceObserver(observer tracepkg.Observer) *Manager {
	if m != nil {
		m.trace = observer
	}
	return m
}

func NewManagerWithGlobalAllowDirs(path string, allowDirs []string) *Manager {
	manager := NewManager(path)
	manager.globalAllowDirs = normalizeRoots(allowDirs)
	return manager
}

func (m *Manager) Reload() error {
	if m == nil {
		return errors.New("workspace manager is unavailable")
	}
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	m.mu.RLock()
	active := m.runtime != nil && m.runtime.active
	currentLocks := map[string]*oslock.Lock{}
	if active {
		for id, lock := range m.runtime.locks {
			currentLocks[id] = lock
		}
	}
	m.mu.RUnlock()
	pendingLocks := map[string]*oslock.Lock{}
	if active {
		candidates, err := m.runtimeLockCandidates()
		if err != nil {
			return err
		}
		for _, item := range candidates {
			if lock := currentLocks[item.ID]; lock != nil {
				same, sameErr := lock.SameFile(workspacestate.New(item.Path).RuntimeLockPath())
				if sameErr == nil && same {
					continue
				}
			}
			lock, err := m.acquireRuntimeLock(item)
			if err != nil {
				_ = releaseRuntimeLocks(pendingLocks)
				return err
			}
			pendingLocks[item.ID] = lock
		}
	}
	fresh := NewManager(m.path).SetTraceObserver(m.trace)
	if active {
		fresh.runtime.preparing = true
	}
	if err := fresh.ensureLoadedState(); err != nil {
		_ = releaseRuntimeLocks(pendingLocks)
		return err
	}
	fresh.mu.RLock()
	items := make(map[string]Workspace, len(fresh.items))
	for id, item := range fresh.items {
		item.AllowDirs = append([]string(nil), item.AllowDirs...)
		item.LegacyIDs = append([]string(nil), item.LegacyIDs...)
		items[id] = item
	}
	containers := make(map[string]WorkspaceContainer, len(fresh.containers))
	for id, container := range fresh.containers {
		container.WorkspaceIDs = append([]string(nil), container.WorkspaceIDs...)
		containers[id] = container
	}
	aliases := make(map[string]string, len(fresh.aliases))
	for alias, canonical := range fresh.aliases {
		aliases[alias] = canonical
	}
	fresh.mu.RUnlock()

	removedLocks := map[string]*oslock.Lock{}
	unusedLocks := map[string]*oslock.Lock{}
	usedPending := map[string]bool{}
	m.mu.Lock()
	if active {
		nextLocks := map[string]*oslock.Lock{}
		nextRoots := map[string]string{}
		for id, item := range items {
			if !item.Available() {
				continue
			}
			if lock := m.runtime.locks[id]; lock != nil {
				same, sameErr := lock.SameFile(workspacestate.New(item.Path).RuntimeLockPath())
				if sameErr == nil && same {
					nextLocks[id] = lock
					nextRoots[id] = item.Path
					continue
				}
			}
			lock := pendingLocks[id]
			if lock == nil {
				m.mu.Unlock()
				_ = releaseRuntimeLocks(pendingLocks)
				return fmt.Errorf("workspace runtime lock missing during reload: %s", id)
			}
			same, sameErr := lock.SameFile(workspacestate.New(item.Path).RuntimeLockPath())
			if sameErr != nil || !same {
				m.mu.Unlock()
				_ = releaseRuntimeLocks(pendingLocks)
				if sameErr != nil {
					return fmt.Errorf("verify workspace runtime lock during reload %s: %w", id, sameErr)
				}
				return fmt.Errorf("workspace runtime lock changed during reload: %s", id)
			}
			nextLocks[id] = lock
			nextRoots[id] = item.Path
			usedPending[id] = true
		}
		for id, lock := range m.runtime.locks {
			if nextLocks[id] != lock {
				removedLocks[id] = lock
			}
		}
		for id, lock := range pendingLocks {
			if !usedPending[id] {
				unusedLocks[id] = lock
			}
		}
		m.runtime.locks = nextLocks
		m.runtime.roots = nextRoots
	}
	m.items = items
	m.containers = containers
	m.aliases = aliases
	m.loaded = true
	m.mu.Unlock()
	return errors.Join(releaseRuntimeLocks(removedLocks), releaseRuntimeLocks(unusedLocks))
}

func (m *Manager) SetGlobalAllowDirs(allowDirs []string) {
	m.mu.Lock()
	m.globalAllowDirs = normalizeRoots(allowDirs)
	m.mu.Unlock()
}

func (m *Manager) SetShellPath(paths []string) {
	m.mu.Lock()
	m.shellPath = normalizeShellPaths(paths)
	m.mu.Unlock()
}

func (m *Manager) ShellPath() []string {
	if m == nil {
		return []string{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.shellPath...)
}

func normalizeShellPaths(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || !filepath.IsAbs(value) {
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
	return result
}

func (m *Manager) Register(path string) (result Workspace, resultErr error) {
	span := tracepkg.StartObserver(m.trace, "WORKSPACE", "workspace.register", "Registering workspace", tracepkg.String("input_path", path))
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	transient, err := m.ensureRuntimeOwnershipLocked()
	if err != nil {
		span.FailMessage("Workspace registration failed", err)
		return Workspace{}, err
	}
	if transient {
		defer func() { resultErr = errors.Join(resultErr, m.deactivateLocked()) }()
	}
	if err := m.ensureLoadedState(); err != nil {
		span.FailMessage("Workspace registration failed", err)
		return Workspace{}, err
	}
	resolveSpan := tracepkg.StartObserver(m.trace, "WORKSPACE", "workspace.path.resolve", "Resolving workspace path", tracepkg.String("input_path", path))
	absolute, _ := filepath.Abs(path)
	root, err := canonicalExistingDirectory(path)
	if err != nil {
		resolveSpan.FailMessage("Workspace path resolution failed", err, tracepkg.String("absolute_path", absolute))
		span.FailMessage("Workspace registration failed", err)
		return Workspace{}, err
	}
	resolveSpan.EndMessage("Workspace path resolved", tracepkg.String("input_path", path), tracepkg.String("absolute_path", absolute), tracepkg.String("canonical_path", root))
	if m.protected(root) {
		err := fmt.Errorf("workspace root is inside protected control-plane state: %s", root)
		span.FailMessage("Workspace registration failed", err, tracepkg.String("canonical_path", root), tracepkg.Bool("protected", true))
		return Workspace{}, err
	}
	local := workspacestate.New(root)
	m.mu.RLock()
	active := m.runtime != nil && m.runtime.active
	activeItem := Workspace{}
	if active {
		for _, registered := range m.items {
			if filepath.Clean(registered.Path) == root {
				activeItem = registered
				break
			}
		}
	}
	m.mu.RUnlock()
	if activeItem.ID != "" {
		if err := validateActiveWorkspaceState(activeItem); err != nil {
			span.FailMessage("Workspace registration failed", err, tracepkg.String("workspace_id", activeItem.ID), tracepkg.String("canonical_path", root))
			return Workspace{}, err
		}
		if err := local.EnsureGitExcluded(context.Background()); err != nil {
			span.FailMessage("Workspace registration failed", err, tracepkg.String("workspace_id", activeItem.ID), tracepkg.String("canonical_path", root))
			return Workspace{}, err
		}
		span.EndMessage("Workspace registered", tracepkg.String("workspace_id", activeItem.ID), tracepkg.String("canonical_path", root), tracepkg.Bool("existing", true), tracepkg.Bool("protected", false), tracepkg.Int("allow_dirs", len(activeItem.AllowDirs)))
		return activeItem, nil
	}
	identity, created, err := local.EnsureIdentity("")
	if err != nil {
		span.FailMessage("Workspace registration failed", err, tracepkg.String("canonical_path", root))
		return Workspace{}, err
	}
	if err := local.EnsureGitExcluded(context.Background()); err != nil {
		span.FailMessage("Workspace registration failed", err, tracepkg.String("workspace_id", identity.ID), tracepkg.String("canonical_path", root))
		return Workspace{}, err
	}
	item := Workspace{ID: identity.ID, Path: root}
	lockedForRegistration := false
	m.mu.Lock()
	existing, existed := m.items[item.ID]
	if existed && filepath.Clean(existing.Path) != root {
		m.mu.Unlock()
		err := fmt.Errorf("workspace identity %s is already registered at %s", item.ID, existing.Path)
		span.FailMessage("Workspace registration failed", err, tracepkg.String("workspace_id", item.ID), tracepkg.String("canonical_path", root))
		return Workspace{}, err
	}
	for id, registered := range m.items {
		if id != item.ID && filepath.Clean(registered.Path) == root {
			m.mu.Unlock()
			err := fmt.Errorf("workspace path is already registered as %s", id)
			span.FailMessage("Workspace registration failed", err, tracepkg.String("workspace_id", item.ID), tracepkg.String("canonical_path", root))
			return Workspace{}, err
		}
	}
	if !existed && m.runtime != nil && m.runtime.active {
		lock, lockErr := m.acquireRuntimeLock(item)
		if lockErr != nil {
			m.mu.Unlock()
			span.FailMessage("Workspace registration failed", lockErr, tracepkg.String("workspace_id", item.ID), tracepkg.String("canonical_path", root))
			return Workspace{}, lockErr
		}
		m.runtime.locks[item.ID] = lock
		m.runtime.roots[item.ID] = item.Path
		lockedForRegistration = true
	}
	m.mu.Unlock()
	releaseRegistrationLock := func() {
		if !lockedForRegistration {
			return
		}
		m.mu.Lock()
		lock := m.releaseRuntimeLockLocked(item.ID)
		m.mu.Unlock()
		if lock != nil {
			_ = lock.Release()
		}
		lockedForRegistration = false
	}
	config := workspacestate.DefaultConfig()
	if !created {
		config, err = local.LoadConfig()
		if err != nil {
			releaseRegistrationLock()
			span.FailMessage("Workspace registration failed", err, tracepkg.String("workspace_id", identity.ID), tracepkg.String("canonical_path", root))
			return Workspace{}, err
		}
	}
	config.AllowDirs = normalizeRoots(config.AllowDirs)
	config.LegacyIDs = normalizeIDs(config.LegacyIDs, identity.ID)
	if created {
		if err := local.SaveConfig(config); err != nil {
			releaseRegistrationLock()
			span.FailMessage("Workspace registration failed", err, tracepkg.String("workspace_id", identity.ID), tracepkg.String("canonical_path", root))
			return Workspace{}, err
		}
	}
	item.AllowDirs = config.AllowDirs
	item.LegacyIDs = config.LegacyIDs

	m.mu.Lock()
	defer m.mu.Unlock()
	existing, existed = m.items[item.ID]
	if existed && filepath.Clean(existing.Path) != root {
		if lockedForRegistration {
			if lock := m.releaseRuntimeLockLocked(item.ID); lock != nil {
				_ = lock.Release()
			}
		}
		err := fmt.Errorf("workspace identity %s is already registered at %s", item.ID, existing.Path)
		span.FailMessage("Workspace registration failed", err, tracepkg.String("workspace_id", item.ID), tracepkg.String("canonical_path", root))
		return Workspace{}, err
	}
	for id, registered := range m.items {
		if id != item.ID && filepath.Clean(registered.Path) == root {
			if lockedForRegistration {
				if lock := m.releaseRuntimeLockLocked(item.ID); lock != nil {
					_ = lock.Release()
				}
			}
			err := fmt.Errorf("workspace path is already registered as %s", id)
			span.FailMessage("Workspace registration failed", err, tracepkg.String("workspace_id", item.ID), tracepkg.String("canonical_path", root))
			return Workspace{}, err
		}
	}
	if !existed && !lockedForRegistration {
		if err := m.acquireRuntimeLockLocked(item); err != nil {
			span.FailMessage("Workspace registration failed", err, tracepkg.String("workspace_id", item.ID), tracepkg.String("canonical_path", root))
			return Workspace{}, err
		}
	}
	m.items[item.ID] = item
	if err := m.saveLocked(); err != nil {
		if existed {
			m.items[item.ID] = existing
		} else {
			delete(m.items, item.ID)
		}
		if !existed || lockedForRegistration {
			if lock := m.releaseRuntimeLockLocked(item.ID); lock != nil {
				_ = lock.Release()
			}
		}
		span.FailMessage("Workspace registration failed", err, tracepkg.String("workspace_id", item.ID), tracepkg.String("canonical_path", root), tracepkg.Bool("existing", existed))
		return Workspace{}, err
	}
	span.EndMessage("Workspace registered", tracepkg.String("workspace_id", item.ID), tracepkg.String("canonical_path", root), tracepkg.Bool("existing", existed), tracepkg.Bool("protected", false), tracepkg.Int("allow_dirs", len(item.AllowDirs)))
	return item, nil
}

func (m *Manager) AddAllowDir(id, path string) (result Workspace, resultErr error) {
	span := tracepkg.StartObserver(m.trace, "WORKSPACE", "workspace.allow-dir.add", "Adding workspace allowed directory", tracepkg.String("workspace_id", id), tracepkg.String("input_path", path))
	resolveSpan := tracepkg.StartObserver(m.trace, "WORKSPACE", "workspace.path.resolve", "Resolving allowed directory", tracepkg.String("workspace_id", id), tracepkg.String("input_path", path))
	absolute, _ := filepath.Abs(path)
	root, err := canonicalExistingDirectory(path)
	if err != nil {
		resolveSpan.FailMessage("Allowed directory resolution failed", err, tracepkg.String("absolute_path", absolute))
		span.FailMessage("Adding workspace allowed directory failed", err)
		return Workspace{}, err
	}
	resolveSpan.EndMessage("Allowed directory resolved", tracepkg.String("absolute_path", absolute), tracepkg.String("canonical_path", root))
	if m.protected(root) {
		err := fmt.Errorf("allowed directory is inside protected control-plane state: %s", root)
		span.FailMessage("Adding workspace allowed directory failed", err, tracepkg.String("canonical_path", root), tracepkg.Bool("protected", true))
		return Workspace{}, err
	}
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	transient, err := m.ensureRuntimeOwnershipLocked()
	if err != nil {
		span.FailMessage("Adding workspace allowed directory failed", err)
		return Workspace{}, err
	}
	if transient {
		defer func() { resultErr = errors.Join(resultErr, m.deactivateLocked()) }()
	}
	if err := m.ensureLoadedState(); err != nil {
		span.FailMessage("Adding workspace allowed directory failed", err)
		return Workspace{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	canonical := m.canonicalIDLocked(id)
	item, ok := m.items[canonical]
	if !ok {
		err := fmt.Errorf("%w: %s", ErrNotFound, id)
		span.FailMessage("Adding workspace allowed directory failed", err)
		return Workspace{}, err
	}
	if err := item.unavailableError(); err != nil {
		span.FailMessage("Adding workspace allowed directory failed", err)
		return Workspace{}, err
	}
	previousCount := len(item.AllowDirs)
	previous := append([]string(nil), item.AllowDirs...)
	item.AllowDirs = normalizeRoots(append(item.AllowDirs, root))
	m.items[canonical] = item
	if err := m.saveWorkspaceConfigLocked(item); err != nil {
		item.AllowDirs = previous
		m.items[canonical] = item
		span.FailMessage("Adding workspace allowed directory failed", err)
		return Workspace{}, err
	}
	span.EndMessage("Workspace allowed directory added", tracepkg.String("workspace_id", canonical), tracepkg.String("canonical_path", root), tracepkg.Bool("protected", false), tracepkg.Int("previous_count", previousCount), tracepkg.Int("count", len(item.AllowDirs)))
	return item, nil
}

func (m *Manager) RemoveAllowDir(id, path string) (result Workspace, resultErr error) {
	span := tracepkg.StartObserver(m.trace, "WORKSPACE", "workspace.allow-dir.remove", "Removing workspace allowed directory", tracepkg.String("workspace_id", id), tracepkg.String("input_path", path))
	absolute, err := filepath.Abs(path)
	if err != nil {
		span.FailMessage("Removing workspace allowed directory failed", err)
		return Workspace{}, err
	}
	root := filepath.Clean(absolute)
	if canonical, err := filepath.EvalSymlinks(root); err == nil {
		root = filepath.Clean(canonical)
	}
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	transient, err := m.ensureRuntimeOwnershipLocked()
	if err != nil {
		span.FailMessage("Removing workspace allowed directory failed", err)
		return Workspace{}, err
	}
	if transient {
		defer func() { resultErr = errors.Join(resultErr, m.deactivateLocked()) }()
	}
	if err := m.ensureLoadedState(); err != nil {
		span.FailMessage("Removing workspace allowed directory failed", err)
		return Workspace{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	canonical := m.canonicalIDLocked(id)
	item, ok := m.items[canonical]
	if !ok {
		err := fmt.Errorf("%w: %s", ErrNotFound, id)
		span.FailMessage("Removing workspace allowed directory failed", err)
		return Workspace{}, err
	}
	if err := item.unavailableError(); err != nil {
		span.FailMessage("Removing workspace allowed directory failed", err)
		return Workspace{}, err
	}
	previousCount := len(item.AllowDirs)
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
		err := fmt.Errorf("workspace allowed directory is not configured: %s", root)
		span.FailMessage("Removing workspace allowed directory failed", err, tracepkg.String("canonical_path", root))
		return Workspace{}, err
	}
	previous := append([]string(nil), item.AllowDirs...)
	item.AllowDirs = normalizeRoots(filtered)
	m.items[canonical] = item
	if err := m.saveWorkspaceConfigLocked(item); err != nil {
		item.AllowDirs = previous
		m.items[canonical] = item
		span.FailMessage("Removing workspace allowed directory failed", err)
		return Workspace{}, err
	}
	span.EndMessage("Workspace allowed directory removed", tracepkg.String("workspace_id", canonical), tracepkg.String("absolute_path", absolute), tracepkg.String("canonical_path", root), tracepkg.Int("previous_count", previousCount), tracepkg.Int("count", len(item.AllowDirs)))
	return item, nil
}

func (m *Manager) EffectiveRoots(id string) ([]string, error) {
	item, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	if err := item.unavailableError(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	global := append([]string(nil), m.globalAllowDirs...)
	m.mu.RUnlock()
	roots := []string{item.Path}
	roots = append(roots, global...)
	roots = append(roots, item.AllowDirs...)
	result := make([]string, 0, len(roots))
	for _, root := range roots {
		root = filepath.Clean(root)
		result = appendUniqueRoot(result, root)
	}
	sort.Strings(result)
	return result, nil
}

func (m *Manager) Get(id string) (Workspace, error) {
	if err := m.ensureLoaded(); err != nil {
		return Workspace{}, err
	}
	m.mu.RLock()
	canonical := m.canonicalIDLocked(id)
	item, ok := m.items[canonical]
	active := m.runtime != nil && m.runtime.active
	m.mu.RUnlock()
	if !ok {
		return Workspace{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if item.Available() && active {
		if err := validateActiveWorkspaceState(item); err != nil {
			return Workspace{}, err
		}
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
	active := m.runtime != nil && m.runtime.active
	items := make([]Workspace, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, item)
	}
	m.mu.RUnlock()
	if active {
		for i, item := range items {
			if !item.Available() {
				continue
			}
			if err := validateActiveWorkspaceState(item); err != nil {
				items[i].Error = err.Error()
			}
		}
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
	if err := item.unavailableError(); err != nil {
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

func (m *Manager) OpenRootForPath(id, path string) (*os.Root, string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, "", errors.New("path is required")
	}
	absolute, err := canonicalForContainment(path, false)
	if err != nil {
		return nil, "", err
	}
	if m.protected(absolute) {
		return nil, "", fmt.Errorf("path is protected: %s", absolute)
	}
	if item, err := m.Get(id); err == nil && within(workspacestate.New(item.Path).Root(), absolute) {
		return nil, "", fmt.Errorf("path is protected workspace state: %s", absolute)
	}
	roots, err := m.EffectiveRoots(id)
	if err != nil {
		return nil, "", err
	}
	selected := ""
	for _, root := range roots {
		root = filepath.Clean(root)
		if !within(root, absolute) {
			continue
		}
		if selected == "" || len(root) > len(selected) {
			selected = root
		}
	}
	if selected == "" {
		return nil, "", fmt.Errorf("path escapes workspace: %s", absolute)
	}
	relative, err := filepath.Rel(selected, absolute)
	if err != nil {
		return nil, "", err
	}
	info, err := os.Lstat(selected)
	if err != nil {
		return nil, "", fmt.Errorf("inspect workspace access root %s: %w", selected, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, "", fmt.Errorf("workspace access root is not a stable directory: %s", selected)
	}
	root, err := os.OpenRoot(selected)
	if err != nil {
		return nil, "", err
	}
	openedInfo, err := root.Stat(".")
	if err != nil || !openedInfo.IsDir() || !os.SameFile(info, openedInfo) {
		_ = root.Close()
		if err != nil {
			return nil, "", fmt.Errorf("verify workspace access root %s: %w", selected, err)
		}
		return nil, "", fmt.Errorf("workspace access root changed while opening: %s", selected)
	}
	return root, relative, nil
}

func (m *Manager) ensureLoaded() error {
	m.mu.RLock()
	loaded := m.loaded
	m.mu.RUnlock()
	if loaded {
		return nil
	}
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	m.mu.RLock()
	loaded = m.loaded
	active := m.runtime != nil && (m.runtime.active || m.runtime.preparing)
	m.mu.RUnlock()
	if loaded {
		return nil
	}
	if active {
		return m.ensureLoadedState()
	}
	if err := m.activateLocked(); err != nil {
		return err
	}
	return m.deactivateLocked()
}

func (m *Manager) ensureLoadedState() error {
	m.mu.RLock()
	if m.loaded {
		m.mu.RUnlock()
		return nil
	}
	m.mu.RUnlock()
	span := tracepkg.StartObserver(m.trace, "WORKSPACE", "workspace.registry.load", "Loading workspace registry", tracepkg.String("path", m.path))

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loaded {
		span.EndMessage("Workspace registry already loaded", tracepkg.Bool("cache_hit", true), tracepkg.Int("workspaces", len(m.items)), tracepkg.Int("containers", len(m.containers)))
		return nil
	}

	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		m.loaded = true
		span.EndMessage("Workspace registry not yet created", tracepkg.Bool("exists", false), tracepkg.Int64("bytes", 0), tracepkg.Int("workspaces", 0), tracepkg.Int("containers", 0))
		return nil
	}
	if err != nil {
		span.FailMessage("Workspace registry load failed", err)
		return fmt.Errorf("read workspace registry: %w", err)
	}

	var stored storeFile
	if err := configformat.UnmarshalPath(m.path, data, &stored); err != nil {
		span.FailMessage("Workspace registry decode failed", err, tracepkg.Int64("bytes", int64(len(data))))
		return fmt.Errorf("decode workspace registry: %w", err)
	}
	if stored.Version < 1 || stored.Version > storeVersion {
		err := fmt.Errorf("unsupported workspace registry version: %d", stored.Version)
		span.FailMessage("Workspace registry validation failed", err, tracepkg.Int64("bytes", int64(len(data))), tracepkg.Int("registry_version", stored.Version))
		return err
	}
	migrated := stored.Version != storeVersion
	unavailable := 0
	for _, storedItem := range stored.Workspaces {
		item, migratedItem, err := m.readIndexedWorkspace(storedItem, stored.Version)
		if err != nil {
			span.FailMessage("Workspace registry validation failed", err, tracepkg.String("workspace_id", storedItem.ID), tracepkg.String("workspace_path", storedItem.Path))
			return err
		}
		if migratedItem {
			migrated = true
		}
		if !item.Available() {
			unavailable++
		}
		if existing, ok := m.items[item.ID]; ok && indexedPath(existing.Path) != indexedPath(item.Path) {
			err := fmt.Errorf("workspace registry id collision: %s", item.ID)
			span.FailMessage("Workspace registry validation failed", err, tracepkg.String("workspace_id", item.ID))
			return err
		}
		itemKey := indexedPath(item.Path)
		for existingID, existing := range m.items {
			if existingID != item.ID && indexedPath(existing.Path) == itemKey {
				err := fmt.Errorf("workspace registry path collision: %s is indexed as both %s and %s", item.Path, existingID, item.ID)
				span.FailMessage("Workspace registry validation failed", err, tracepkg.String("workspace_id", item.ID))
				return err
			}
		}
		m.items[item.ID] = item
		for _, alias := range item.LegacyIDs {
			if err := m.registerAliasLocked(alias, item.ID); err != nil {
				span.FailMessage("Workspace registry alias validation failed", err, tracepkg.String("workspace_id", item.ID), tracepkg.String("legacy_workspace_id", alias))
				return err
			}
		}
	}
	for _, container := range stored.Containers {
		container.ID = strings.TrimSpace(container.ID)
		container.Name = strings.TrimSpace(container.Name)
		if container.ID == "" || container.Name == "" || !strings.HasPrefix(container.ID, "wsc_") {
			err := errors.New("workspace registry contains invalid container entry")
			span.FailMessage("Workspace registry container validation failed", err, tracepkg.String("container_id", container.ID))
			return err
		}
		if _, exists := m.containers[container.ID]; exists {
			err := fmt.Errorf("workspace container id collision: %s", container.ID)
			span.FailMessage("Workspace registry container validation failed", err, tracepkg.String("container_id", container.ID))
			return err
		}
		container.WorkspaceIDs = normalizeContainerWorkspaceIDs(container.WorkspaceIDs, m.items)
		m.containers[container.ID] = container
	}
	m.loaded = true
	if migrated {
		if err := m.saveLocked(); err != nil {
			m.loaded = false
			span.FailMessage("Migrated workspace registry persistence failed", err, tracepkg.Int("registry_version", stored.Version), tracepkg.Int("target_version", storeVersion))
			return fmt.Errorf("persist migrated workspace registry: %w", err)
		}
	}
	format := ""
	if detected, detectErr := configformat.Detect(m.path); detectErr == nil {
		format = string(detected)
	}
	span.EndMessage("Workspace registry loaded", tracepkg.Bool("exists", true), tracepkg.String("format", format), tracepkg.Int64("bytes", int64(len(data))), tracepkg.Int("registry_version", stored.Version), tracepkg.Int("current_version", storeVersion), tracepkg.Int("workspaces", len(m.items)), tracepkg.Int("unavailable", unavailable), tracepkg.Int("containers", len(m.containers)), tracepkg.Bool("migrated", migrated))
	return nil
}

func (m *Manager) readIndexedWorkspace(stored storedWorkspace, version int) (Workspace, bool, error) {
	id := strings.TrimSpace(stored.ID)
	path := strings.TrimSpace(stored.Path)
	if id == "" || path == "" {
		return Workspace{}, false, errors.New("workspace registry contains invalid entry")
	}
	indexPath := indexedPath(path)
	if m.protected(indexPath) {
		return unavailableWorkspace(id, indexPath, fmt.Errorf("workspace root is inside protected control-plane state: %s", path), stored.LegacyIDs), false, nil
	}
	root, err := canonicalExistingDirectory(path)
	if err != nil {
		return unavailableWorkspace(id, indexPath, err, stored.LegacyIDs), false, nil
	}
	local := workspacestate.New(root)
	identity, err := local.LoadIdentity()
	created := false
	if errors.Is(err, os.ErrNotExist) && version < storeVersion {
		identity, created, err = local.EnsureIdentity(id)
		if err == nil {
			err = local.EnsureGitExcluded(context.Background())
		}
	}
	if err != nil {
		return unavailableWorkspace(id, root, err, stored.LegacyIDs), false, nil
	}
	if identity.ID != id {
		return unavailableWorkspace(id, root, fmt.Errorf("workspace identity mismatch: local %s, expected %s", identity.ID, id), stored.LegacyIDs), false, nil
	}
	config := workspacestate.DefaultConfig()
	if !created {
		config, err = local.LoadConfig()
		if version < storeVersion && errors.Is(err, os.ErrNotExist) {
			config = workspacestate.DefaultConfig()
			created = true
			err = nil
		}
		if err != nil {
			return unavailableWorkspace(id, root, err, stored.LegacyIDs), false, nil
		}
	}
	migrated := version < storeVersion || len(stored.AllowDirs) > 0 || len(stored.LegacyIDs) > 0 || created
	if migrated {
		config.AllowDirs = normalizeRoots(append(config.AllowDirs, stored.AllowDirs...))
		config.LegacyIDs = normalizeIDs(append(config.LegacyIDs, stored.LegacyIDs...), identity.ID)
		if err := local.SaveConfig(config); err != nil {
			return unavailableWorkspace(id, root, err, stored.LegacyIDs), false, nil
		}
	}
	legacyRoots := append([]string{stored.ID}, stored.LegacyIDs...)
	for _, legacyID := range legacyRoots {
		legacyStatePath := filepath.Join(filepath.Dir(m.path), "workspaces", legacyID)
		if err := local.MigrateLegacyState(legacyStatePath); err != nil {
			return unavailableWorkspace(id, root, err, stored.LegacyIDs), migrated, nil
		}
	}
	if err := local.EnsureGitExcluded(context.Background()); err != nil {
		return unavailableWorkspace(id, root, err, stored.LegacyIDs), migrated, nil
	}
	return Workspace{ID: identity.ID, Path: root, AllowDirs: normalizeRoots(config.AllowDirs), LegacyIDs: normalizeIDs(config.LegacyIDs, identity.ID)}, migrated, nil
}

func unavailableWorkspace(id, path string, err error, legacyIDs []string) Workspace {
	message := "unavailable"
	if err != nil {
		message = err.Error()
	}
	return Workspace{ID: id, Path: path, Error: message, LegacyIDs: normalizeIDs(legacyIDs, id)}
}

func indexedPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
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

func (m *Manager) allowed(id, candidate string) bool {
	if m.protected(candidate) {
		return false
	}
	if item, err := m.Get(id); err == nil && within(workspacestate.New(item.Path).Root(), candidate) {
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
	span := tracepkg.StartObserver(m.trace, "WORKSPACE", "workspace.registry.persist", "Persisting workspace registry", tracepkg.String("path", m.path), tracepkg.Int("workspaces", len(m.items)), tracepkg.Int("containers", len(m.containers)), tracepkg.Bool("atomic", true))
	items := make([]storedWorkspace, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, storedWorkspace{ID: item.ID, Path: item.Path})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	containers := make([]WorkspaceContainer, 0, len(m.containers))
	for _, container := range m.containers {
		container.WorkspaceIDs = normalizeContainerWorkspaceIDs(container.WorkspaceIDs, m.items)
		containers = append(containers, container)
	}
	sort.Slice(containers, func(i, j int) bool {
		left, right := strings.ToLower(containers[i].Name), strings.ToLower(containers[j].Name)
		if left == right {
			return containers[i].ID < containers[j].ID
		}
		return left < right
	})
	data, err := configformat.MarshalPath(m.path, storeFile{Version: storeVersion, Workspaces: items, Containers: containers})
	if err != nil {
		span.FailMessage("Workspace registry encoding failed", err)
		return err
	}
	if err := state.WriteFileAtomic(m.path, data, 0600); err != nil {
		span.FailMessage("Workspace registry persistence failed", err, tracepkg.Int64("bytes", int64(len(data))))
		return err
	}
	format := ""
	if detected, detectErr := configformat.Detect(m.path); detectErr == nil {
		format = string(detected)
	}
	span.EndMessage("Workspace registry persisted", tracepkg.String("format", format), tracepkg.Int64("bytes", int64(len(data))), tracepkg.Int("registry_version", storeVersion), tracepkg.Int("workspaces", len(items)), tracepkg.Int("containers", len(containers)))
	return nil
}

func (m *Manager) saveWorkspaceConfigLocked(item Workspace) error {
	if err := item.unavailableError(); err != nil {
		return err
	}
	if m.runtime != nil && m.runtime.active {
		if err := validateActiveWorkspaceState(item); err != nil {
			return err
		}
	}
	return workspacestate.New(item.Path).SaveConfig(workspacestate.Config{AllowDirs: normalizeRoots(item.AllowDirs), LegacyIDs: normalizeIDs(item.LegacyIDs, item.ID)})
}

func (m *Manager) LocalState(id string) (workspacestate.Store, error) {
	item, err := m.Get(id)
	if err != nil {
		return workspacestate.Store{}, err
	}
	if err := item.unavailableError(); err != nil {
		return workspacestate.Store{}, err
	}
	return workspacestate.New(item.Path), nil
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
		if _, err := os.Lstat(current); err == nil { // #nosec G703 -- current is an absolute path walked upward only to find the nearest existing ancestor.
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

func workspaceContainerID() (string, error) {
	return idgen.New("wsc", 8)
}

func IDForPath(path string) string { return workspaceID(path) }

func instanceScopedWorkspaceID(instanceID, path string) string {
	sum := sha256.Sum256([]byte(instanceID + "\x00" + normalizeForID(path)))
	return "ws_" + hex.EncodeToString(sum[:])[:16]
}
