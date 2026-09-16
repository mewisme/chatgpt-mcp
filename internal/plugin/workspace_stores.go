package plugin

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
)

type WorkspaceStores struct {
	runtime RuntimeContext
	mu      sync.Mutex
	stores  map[string]*Store
}

func NewWorkspaceStores(runtime RuntimeContext) *WorkspaceStores {
	return &WorkspaceStores{runtime: runtime, stores: map[string]*Store{}}
}

func (stores *WorkspaceStores) Load(id, root string) (*Store, ReconcileReport, error) {
	id = strings.TrimSpace(id)
	root = filepath.Clean(strings.TrimSpace(root))
	if id == "" {
		return nil, ReconcileReport{}, errors.New("workspace id is required")
	}
	if stores == nil {
		return nil, ReconcileReport{}, errors.New("workspace plugin stores are unavailable")
	}
	stores.mu.Lock()
	defer stores.mu.Unlock()
	if store, ok := stores.stores[id]; ok && sameCleanPath(store.layout.WorkspaceRoot, root) {
		return store, ReconcileReport{}, nil
	}
	layout, err := WorkspaceLayout(root)
	if err != nil {
		return nil, ReconcileReport{}, err
	}
	store, err := NewStore(layout, stores.runtime)
	if err != nil {
		return nil, ReconcileReport{}, err
	}
	report, err := Reconcile(store)
	if err != nil {
		return nil, ReconcileReport{}, err
	}
	if stores.stores == nil {
		stores.stores = map[string]*Store{}
	}
	stores.stores[id] = store
	return store, report, nil
}

func (stores *WorkspaceStores) Unload(id string) {
	if stores == nil {
		return
	}
	stores.mu.Lock()
	delete(stores.stores, strings.TrimSpace(id))
	stores.mu.Unlock()
}

func (stores *WorkspaceStores) Get(id string) (*Store, bool) {
	if stores == nil {
		return nil, false
	}
	stores.mu.Lock()
	defer stores.mu.Unlock()
	store, ok := stores.stores[strings.TrimSpace(id)]
	return store, ok
}

func (stores *WorkspaceStores) All() []*Store {
	if stores == nil {
		return nil
	}
	stores.mu.Lock()
	defer stores.mu.Unlock()
	all := make([]*Store, 0, len(stores.stores))
	for _, store := range stores.stores {
		all = append(all, store)
	}
	return all
}

func (stores *WorkspaceStores) SetGlobalPeer(global *Store) {
	if stores == nil {
		return
	}
	peers := stores.All()
	if global != nil {
		global.SetPeers(peers...)
	}
	for _, store := range peers {
		store.SetPeers(global)
	}
}
