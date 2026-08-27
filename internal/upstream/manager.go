package upstream

import "sync"

type Manager struct {
	mu      sync.RWMutex
	store   *Store
	servers map[string]Server
}

func NewManager(store *Store) *Manager { return &Manager{store: store, servers: map[string]Server{}} }

func (m *Manager) Add(server Server) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers[server.ID] = server
	return m.persist()
}

func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.servers, id)
	return m.persist()
}

func (m *Manager) List() []Server {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Server, 0, len(m.servers))
	for _, server := range m.servers {
		out = append(out, server)
	}
	return out
}

func (m *Manager) persist() error {
	if m.store == nil {
		return nil
	}
	servers := make([]Server, 0, len(m.servers))
	for _, server := range m.servers {
		servers = append(servers, server)
	}
	return m.store.Save(servers)
}
