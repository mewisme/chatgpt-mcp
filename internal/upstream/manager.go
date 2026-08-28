package upstream

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const toolsCacheTTL = 60 * time.Second

type Health string

const (
	HealthUnknown     Health = "unknown"
	HealthConnected   Health = "connected"
	HealthUnreachable Health = "unreachable"
	HealthDisabled    Health = "disabled"
)

type Status struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	Transport    string   `json:"transport"`
	Auth         string   `json:"auth"`
	Health       Health   `json:"health"`
	Connected    bool     `json:"connected"`
	ToolCount    int      `json:"tool_count"`
	Expose       string   `json:"expose"`
	ProxiedTools []string `json:"proxied_tools"`
	LastError    string   `json:"last_error,omitempty"`
	PID          *int     `json:"pid,omitempty"`
}

type toolCache struct {
	tools     []Tool
	expiresAt time.Time
}

type Manager struct {
	mu      sync.RWMutex
	store   *Store
	client  Client
	servers map[string]Server
	cache   map[string]toolCache
	errors  map[string]string
}

func NewManager(store *Store) *Manager {
	return NewManagerWithClient(store, NewNativeClient())
}

func NewManagerWithClient(store *Store, client Client) *Manager {
	return &Manager{store: store, client: client, servers: map[string]Server{}, cache: map[string]toolCache{}, errors: map[string]string{}}
}

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
		normalized, err := NormalizeServer(server)
		if err != nil {
			return err
		}
		next[normalized.ID] = normalized
	}
	m.mu.Lock()
	m.servers = next
	m.cache = map[string]toolCache{}
	m.errors = map[string]string{}
	m.mu.Unlock()
	return nil
}

func (m *Manager) Reload(ctx context.Context) error {
	if err := m.Shutdown(ctx); err != nil {
		return err
	}
	return m.Load()
}

func (m *Manager) Add(server Server) error {
	normalized, err := NormalizeServer(server)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	previous, existed := m.servers[normalized.ID]
	m.servers[normalized.ID] = normalized
	if err := m.persistLocked(); err != nil {
		if existed {
			m.servers[normalized.ID] = previous
		} else {
			delete(m.servers, normalized.ID)
		}
		return err
	}
	delete(m.cache, normalized.ID)
	delete(m.errors, normalized.ID)
	return nil
}

func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	previous, existed := m.servers[id]
	delete(m.servers, id)
	if err := m.persistLocked(); err != nil {
		if existed {
			m.servers[id] = previous
		}
		m.mu.Unlock()
		return err
	}
	delete(m.cache, id)
	delete(m.errors, id)
	m.mu.Unlock()
	return m.client.Close(context.Background(), id)
}

func (m *Manager) Get(id string) (Server, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.servers[id]
	return value, ok
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

func (m *Manager) Tools(ctx context.Context, id string, force bool) ([]Tool, error) {
	server, ok := m.Get(id)
	if !ok {
		return nil, errors.New("unknown upstream server: " + id)
	}
	if !server.Enabled {
		return nil, errors.New("upstream server disabled: " + id)
	}
	m.mu.RLock()
	cached, hasCache := m.cache[id]
	m.mu.RUnlock()
	if !force && hasCache && time.Now().Before(cached.expiresAt) {
		return append([]Tool(nil), cached.tools...), nil
	}
	if force {
		_ = m.client.Close(context.Background(), id)
	}
	if err := m.client.Connect(ctx, server); err != nil {
		m.recordError(id, err)
		return nil, err
	}
	tools, err := m.client.Tools(ctx, id)
	if err != nil {
		m.recordError(id, err)
		return nil, err
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	m.mu.Lock()
	m.cache[id] = toolCache{tools: append([]Tool(nil), tools...), expiresAt: time.Now().Add(toolsCacheTTL)}
	delete(m.errors, id)
	m.mu.Unlock()
	return tools, nil
}

func (m *Manager) Call(ctx context.Context, id, tool string, args map[string]any) (CallResult, error) {
	server, ok := m.Get(id)
	if !ok {
		return CallResult{}, errors.New("unknown upstream server: " + id)
	}
	if !server.Enabled {
		return CallResult{}, errors.New("upstream server disabled: " + id)
	}
	if err := m.client.Connect(ctx, server); err != nil {
		m.recordError(id, err)
		return CallResult{}, err
	}
	result, err := m.client.Call(ctx, id, tool, args)
	if err != nil {
		m.recordError(id, err)
		return CallResult{}, err
	}
	m.mu.Lock()
	delete(m.errors, id)
	m.mu.Unlock()
	return result, nil
}

func (m *Manager) CheckHealth(ctx context.Context, id string, force bool) Status {
	server, ok := m.Get(id)
	if !ok {
		return Status{ID: id, Health: HealthUnreachable, LastError: "unknown upstream server"}
	}
	if !server.Enabled {
		return m.buildStatus(server, HealthDisabled, false, nil, "")
	}
	tools, err := m.Tools(ctx, id, force)
	if err != nil {
		return m.buildStatus(server, HealthUnreachable, false, nil, err.Error())
	}
	return m.buildStatus(server, HealthConnected, true, tools, "")
}

func (m *Manager) ListStatuses(ctx context.Context, refresh bool) []Status {
	servers := m.List()
	result := make([]Status, len(servers))
	var wg sync.WaitGroup
	for index, server := range servers {
		index, server := index, server
		wg.Add(1)
		go func() {
			defer wg.Done()
			result[index] = m.CheckHealth(ctx, server.ID, refresh)
		}()
	}
	wg.Wait()
	return result
}

func (m *Manager) ProxiedToolNames(server Server, tools []Tool) []string {
	if !server.Enabled || server.Expose == "none" || server.Expose == "meta_only" {
		return []string{}
	}
	allow := stringSet(server.Tools)
	deny := stringSet(server.DisabledTools)
	result := make([]string, 0)
	for _, tool := range tools {
		if deny[tool.Name] {
			continue
		}
		if server.Expose == "allowlist" && !allow[tool.Name] {
			continue
		}
		result = append(result, ProxyName(server.ToolPrefix, tool.Name))
	}
	sort.Strings(result)
	return result
}

func (m *Manager) Shutdown(ctx context.Context) error {
	servers := m.List()
	var first error
	for _, server := range servers {
		if err := m.client.Close(ctx, server.ID); err != nil && first == nil {
			first = err
		}
	}
	m.mu.Lock()
	m.cache = map[string]toolCache{}
	m.mu.Unlock()
	return first
}

func (m *Manager) buildStatus(server Server, health Health, connected bool, tools []Tool, lastError string) Status {
	proxied := m.ProxiedToolNames(server, tools)
	auth := "none"
	if server.Transport == "http" {
		if len(server.Headers) > 0 || server.BearerTokenEnvVar != "" {
			auth = "static"
		} else if server.Auth.Type == "oauth" || server.Auth.Type == "auto" {
			auth = "oauth"
		}
	}
	status := Status{
		ID: server.ID, Name: server.Name, Enabled: server.Enabled, Transport: server.Transport,
		Auth: auth, Health: health, Connected: connected, ToolCount: len(tools),
		Expose: server.Expose, ProxiedTools: proxied, LastError: lastError,
	}
	if pid := m.client.PID(server.ID); pid > 0 {
		status.PID = &pid
	}
	return status
}

func (m *Manager) recordError(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[id] = err.Error()
}

func (m *Manager) persistLocked() error {
	if m.store == nil {
		return nil
	}
	return m.store.Save(m.listLocked())
}

func stringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = true
		}
	}
	return result
}
