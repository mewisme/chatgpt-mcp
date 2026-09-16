package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/oslock"
	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
)

var (
	ErrAlreadyActive = errors.New("workspace already active")
	ErrStateLost     = errors.New("workspace state lost or replaced while workspace is active")
)

type runtimeState struct {
	active    bool
	preparing bool
	locks     map[string]*oslock.Lock
	roots     map[string]string
}

type runtimeLockMetadata struct {
	PID         int       `json:"pid"`
	InstanceID  string    `json:"instance_id,omitempty"`
	WorkspaceID string    `json:"workspace_id"`
	StartedAt   time.Time `json:"started_at"`
}

func newRuntimeState() *runtimeState {
	return &runtimeState{locks: map[string]*oslock.Lock{}, roots: map[string]string{}}
}

func (m *Manager) Activate() error {
	if m == nil {
		return errors.New("workspace manager is unavailable")
	}
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	return m.activateLocked()
}

func (m *Manager) activateLocked() error {
	m.mu.RLock()
	if m.runtime == nil {
		m.mu.RUnlock()
		m.mu.Lock()
		if m.runtime == nil {
			m.runtime = newRuntimeState()
		}
		m.mu.Unlock()
		m.mu.RLock()
	}
	if m.runtime.active {
		m.mu.RUnlock()
		return nil
	}
	m.mu.RUnlock()
	candidates, err := m.runtimeLockCandidates()
	if err != nil {
		return err
	}
	locks := map[string]*oslock.Lock{}
	roots := map[string]string{}
	for _, item := range candidates {
		lock, err := m.acquireRuntimeLock(item)
		if err != nil {
			return errors.Join(err, releaseRuntimeLocks(locks))
		}
		locks[item.ID] = lock
		roots[item.ID] = item.Path
	}
	m.mu.Lock()
	m.runtime.preparing = true
	m.runtime.locks = locks
	m.runtime.roots = roots
	m.mu.Unlock()
	if err := m.ensureLoadedState(); err != nil {
		m.resetRuntimeActivation()
		return err
	}
	m.mu.Lock()
	if err := m.reconcileRuntimeLocksLocked(m.items); err != nil {
		m.mu.Unlock()
		m.resetRuntimeActivation()
		return err
	}
	m.runtime.preparing = false
	m.runtime.active = true
	m.mu.Unlock()
	return nil
}

func (m *Manager) Deactivate() error {
	if m == nil {
		return nil
	}
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	return m.deactivateLocked()
}

func (m *Manager) deactivateLocked() error {
	m.mu.Lock()
	if m.runtime == nil || (!m.runtime.active && !m.runtime.preparing) {
		m.mu.Unlock()
		return nil
	}
	locks := m.runtime.locks
	m.runtime = newRuntimeState()
	m.mu.Unlock()
	return releaseRuntimeLocks(locks)
}

func (m *Manager) Active() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runtime != nil && m.runtime.active
}

func (m *Manager) ensureRuntimeOwnershipLocked() (bool, error) {
	m.mu.RLock()
	active := m.runtime != nil && m.runtime.active
	m.mu.RUnlock()
	if active {
		return false, nil
	}
	if err := m.activateLocked(); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) acquireRuntimeLockLocked(item Workspace) error {
	if m.runtime == nil || !m.runtime.active {
		return nil
	}
	if _, ok := m.runtime.locks[item.ID]; ok {
		return validateActiveWorkspaceState(item)
	}
	if err := validateActiveWorkspaceState(item); err != nil {
		return err
	}
	lock, err := m.acquireRuntimeLock(item)
	if err != nil {
		return err
	}
	m.runtime.locks[item.ID] = lock
	m.runtime.roots[item.ID] = item.Path
	return nil
}

func (m *Manager) releaseRuntimeLockLocked(id string) *oslock.Lock {
	if m.runtime == nil {
		return nil
	}
	lock := m.runtime.locks[id]
	delete(m.runtime.locks, id)
	delete(m.runtime.roots, id)
	return lock
}

func (m *Manager) reconcileRuntimeLocksLocked(items map[string]Workspace) error {
	if m.runtime == nil || (!m.runtime.active && !m.runtime.preparing) {
		return nil
	}
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	nextLocks := map[string]*oslock.Lock{}
	nextRoots := map[string]string{}
	newLocks := map[string]*oslock.Lock{}
	for _, id := range ids {
		item := items[id]
		if !item.Available() {
			continue
		}
		if err := validateActiveWorkspaceState(item); err != nil {
			return errors.Join(err, releaseRuntimeLocks(newLocks))
		}
		if lock := m.runtime.locks[id]; lock != nil {
			same, sameErr := lock.SameFile(workspacestate.New(item.Path).RuntimeLockPath())
			if sameErr == nil && same {
				nextLocks[id] = lock
				nextRoots[id] = item.Path
				continue
			}
		}
		lock, err := m.acquireRuntimeLock(item)
		if err != nil {
			return errors.Join(err, releaseRuntimeLocks(newLocks))
		}
		newLocks[id] = lock
		nextLocks[id] = lock
		nextRoots[id] = item.Path
	}
	removed := map[string]*oslock.Lock{}
	for id, lock := range m.runtime.locks {
		if nextLocks[id] != lock {
			removed[id] = lock
		}
	}
	m.runtime.locks = nextLocks
	m.runtime.roots = nextRoots
	return releaseRuntimeLocks(removed)
}

func (m *Manager) runtimeLockCandidates() ([]Workspace, error) {
	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workspace registry: %w", err)
	}
	var stored storeFile
	if err := configformat.UnmarshalPath(m.path, data, &stored); err != nil {
		return nil, fmt.Errorf("decode workspace registry: %w", err)
	}
	if stored.Version < 1 || stored.Version > storeVersion {
		return nil, fmt.Errorf("unsupported workspace registry version: %d", stored.Version)
	}
	items := make([]Workspace, 0, len(stored.Workspaces))
	seenIDs := map[string]string{}
	seenRoots := map[string]string{}
	for _, storedItem := range stored.Workspaces {
		id := strings.TrimSpace(storedItem.ID)
		path := strings.TrimSpace(storedItem.Path)
		if id == "" || path == "" {
			return nil, errors.New("workspace registry contains invalid entry")
		}
		indexPath := indexedPath(path)
		if _, exists := seenIDs[id]; exists {
			return nil, fmt.Errorf("workspace registry id collision: %s", id)
		}
		if otherID := seenRoots[indexPath]; otherID != "" && otherID != id {
			return nil, fmt.Errorf("workspace registry path collision: %s is indexed as both %s and %s", path, otherID, id)
		}
		seenIDs[id] = path
		seenRoots[indexPath] = id
		root, err := canonicalExistingDirectory(path)
		if err != nil {
			continue
		}
		if m.protected(root) {
			continue
		}
		local := workspacestate.New(root)
		identity, err := local.LoadIdentity()
		if errors.Is(err, os.ErrNotExist) && stored.Version < storeVersion {
			identity, _, err = local.EnsureIdentity(id)
			if err == nil {
				err = local.EnsureGitExcluded(context.Background())
			}
		}
		if err != nil || identity.ID != id {
			continue
		}
		items = append(items, Workspace{ID: id, Path: root})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Path == items[j].Path {
			return items[i].ID < items[j].ID
		}
		return items[i].Path < items[j].Path
	})
	return items, nil
}

func (m *Manager) resetRuntimeActivation() {
	m.mu.Lock()
	locks := m.runtime.locks
	m.runtime = newRuntimeState()
	m.mu.Unlock()
	_ = releaseRuntimeLocks(locks)
}

func (m *Manager) acquireRuntimeLock(item Workspace) (*oslock.Lock, error) {
	lock, ok, err := oslock.TryAcquire(workspacestate.New(item.Path).RuntimeLockPath(), oslock.Exclusive)
	if err != nil {
		return nil, fmt.Errorf("lock workspace %s: %w", item.ID, err)
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAlreadyActive, item.ID)
	}
	identity, _ := m.Instance()
	metadata, err := json.MarshalIndent(runtimeLockMetadata{PID: os.Getpid(), InstanceID: identity.ID, WorkspaceID: item.ID, StartedAt: time.Now().UTC()}, "", "  ")
	if err != nil {
		_ = lock.Release()
		return nil, err
	}
	if err := lock.ReplaceContent(append(metadata, '\n')); err != nil {
		_ = lock.Release()
		return nil, fmt.Errorf("write workspace %s lock metadata: %w", item.ID, err)
	}
	return lock, nil
}

func validateActiveWorkspaceState(item Workspace) error {
	local := workspacestate.New(item.Path)
	identity, err := local.LoadIdentity()
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrStateLost, item.ID, err)
	}
	if identity.ID != item.ID {
		return fmt.Errorf("%w: %s: identity changed to %s", ErrStateLost, item.ID, identity.ID)
	}
	if _, err := local.LoadConfig(); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrStateLost, item.ID, err)
	}
	return nil
}

func releaseRuntimeLocks(locks map[string]*oslock.Lock) error {
	var result error
	for _, lock := range locks {
		result = errors.Join(result, lock.Release())
	}
	return result
}
