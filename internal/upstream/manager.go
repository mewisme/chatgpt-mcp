package upstream

import (
	"sort"
	"sync"
)

type Manager struct {
	mu      sync.RWMutex
	store   *Store
	servers map[string]Server
}

func NewManager(store *Store) *Manager { return &Manager{store: store, servers: map[string]Server{}} }

func (m *Manager) Load() error {
	if m.store == nil {
		return nil
	}
	servers, err := m.store.Load()
	if err != nil {
		return err
	}
	next := make(map[string]Server, len(servers))
	for _, server := range servers {
		next[server.ID] = server
	}
	m.mu.Lock()
	m.servers = next
	m.mu.Unlock()
	return nil
}

func (m *Manager) Add(server Server) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	previous, existed := m.servers[server.ID]
	m.servers[server.ID] = server
	if err := m.persistLocked(); err != nil {
		if existed {
			m.servers[server.ID] = previous
		} else {
			delete(m.servers, server.ID)
		}
		return err
	}
	return nil
}

func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	previous, existed := m.servers[id]
	delete(m.servers, id)
	if err := m.persistLocked(); err != nil {
		if existed {
			m.servers[id] = previous
		}
		return err
	}
	return nil
}

func (m *Manager) List() []Server {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked()
}

func (m *Manager) listLocked() []Server {
	out := make([]Server, 0, len(m.servers))
	for _, server := range m.servers {
		out = append(out, server)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *Manager) persistLocked() error {
	if m.store == nil {
		return nil
	}
	return m.store.Save(m.listLocked())
}
