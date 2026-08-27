package upstream

import "sync"

type Server struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Enabled   bool   `json:"enabled"`
}

type Manager struct {
	mu      sync.RWMutex
	servers map[string]Server
}

func NewManager() *Manager           { return &Manager{servers: map[string]Server{}} }
func (m *Manager) Add(server Server) { m.mu.Lock(); defer m.mu.Unlock(); m.servers[server.ID] = server }
func (m *Manager) List() []Server {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Server, 0, len(m.servers))
	for _, s := range m.servers {
		out = append(out, s)
	}
	return out
}
