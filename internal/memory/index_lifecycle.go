package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
)

type IndexLifecycle struct {
	Store        Store
	Index        Index
	mu           sync.Mutex
	fingerprints map[string]string
}

func NewIndexLifecycle(store Store, index Index) *IndexLifecycle {
	return &IndexLifecycle{Store: store, Index: index, fingerprints: map[string]string{}}
}

func (l *IndexLifecycle) Ensure(workspaceID string) error {
	if l == nil || l.Index == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fingerprint, err := l.fingerprint(workspaceID)
	if err != nil {
		return err
	}
	if current, ok := l.fingerprints[workspaceID]; ok && current == fingerprint {
		return nil
	}
	document, err := l.Store.LoadDocument(workspaceID)
	if err != nil {
		return err
	}
	if err := l.Index.Rebuild(workspaceID, document.Entries); err != nil {
		return err
	}
	l.fingerprints[workspaceID] = fingerprint
	return nil
}

func (l *IndexLifecycle) Invalidate(workspaceID string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	delete(l.fingerprints, workspaceID)
	l.mu.Unlock()
}

func (l *IndexLifecycle) fingerprint(workspaceID string) (string, error) {
	data, err := os.ReadFile(l.Store.WorkspacePath(workspaceID))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
