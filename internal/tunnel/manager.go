package tunnel

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

// InstanceConfig contains only the credentials and settings needed to run an ingress.
type InstanceConfig struct {
	Enabled             bool   `json:"enabled"`
	ID                  string `json:"id"`
	APIKey              string `json:"-"`
	AdminProfileID      string `json:"admin_profile_id,omitempty"`
	ControlPlaneBaseURL string `json:"control_plane_base_url,omitempty"`
	OrganizationID      string `json:"organization_id,omitempty"`
}

// AdminConfig is a management credential; it does not own a runtime connection.
type AdminConfig struct {
	ID                  string `json:"id"`
	AdminKey            string `json:"-"`
	OrganizationID      string `json:"organization_id,omitempty"`
	WorkspaceID         string `json:"workspace_id,omitempty"`
	TenantID            string `json:"tenant_id,omitempty"`
	ReadAccess          bool   `json:"read_access,omitempty"`
	ManageAccess        bool   `json:"manage_access,omitempty"`
	ControlPlaneBaseURL string `json:"control_plane_base_url,omitempty"`
}

type CollectionConfig struct {
	Instances []InstanceConfig `json:"instances"`
	Admins    []AdminConfig    `json:"admins"`
}

func (cfg CollectionConfig) Validate() error {
	profiles := make(map[string]bool, len(cfg.Admins))
	for _, admin := range cfg.Admins {
		if strings.TrimSpace(admin.ID) == "" {
			return errors.New("tunnel admin profile ID is empty")
		}
		if profiles[admin.ID] {
			return fmt.Errorf("duplicate tunnel admin profile %q", admin.ID)
		}
		profiles[admin.ID] = true
	}
	instances := make(map[string]bool, len(cfg.Instances))
	for _, instance := range cfg.Instances {
		if strings.TrimSpace(instance.ID) == "" {
			return errors.New("tunnel instance ID is empty")
		}
		if instances[instance.ID] {
			return fmt.Errorf("duplicate tunnel instance %q", instance.ID)
		}
		instances[instance.ID] = true
		if instance.AdminProfileID != "" && !profiles[instance.AdminProfileID] {
			return fmt.Errorf("tunnel %q references unknown admin profile %q", instance.ID, instance.AdminProfileID)
		}
		if err := ValidateConfig(instance.clientConfig()); err != nil {
			return fmt.Errorf("tunnel %q: %w", instance.ID, err)
		}
	}
	return nil
}

func (cfg InstanceConfig) clientConfig() Config {
	return Config{Enabled: cfg.Enabled, ID: cfg.ID, APIKey: cfg.APIKey, ControlPlaneBaseURL: cfg.ControlPlaneBaseURL, OrganizationID: cfg.OrganizationID}
}

// Manager owns independent tunnel transports pointing at exactly one tools runtime.
type Manager struct {
	mu      sync.RWMutex
	runtime *tools.Runtime
	logger  *logger.Logger
	clients map[string]*Client
	configs map[string]InstanceConfig
	running bool
}

func NewManager(runtime *tools.Runtime, log *logger.Logger) *Manager {
	return &Manager{runtime: runtime, logger: log, clients: make(map[string]*Client), configs: make(map[string]InstanceConfig)}
}

func (m *Manager) Client(id string) (*Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	client, ok := m.clients[id]
	return client, ok
}

func (m *Manager) Statuses() []Status {
	m.mu.RLock()
	ids := make([]string, 0, len(m.clients))
	for id := range m.clients {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	clients := make([]*Client, 0, len(ids))
	for _, id := range ids {
		clients = append(clients, m.clients[id])
	}
	m.mu.RUnlock()
	statuses := make([]Status, 0, len(clients))
	for _, client := range clients {
		statuses = append(statuses, client.Status())
	}
	return statuses
}

func (m *Manager) Ready() bool {
	for _, status := range m.Statuses() {
		if status.Enabled && status.Ready {
			return true
		}
	}
	return false
}

func (m *Manager) StartContext(ctx context.Context) error {
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()
	var errs []error
	for _, status := range m.Statuses() {
		if status.Enabled {
			if client, ok := m.Client(status.ID); ok {
				if err := client.StartContext(ctx); err != nil {
					errs = append(errs, fmt.Errorf("tunnel %s: %w", status.ID, err))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) StopContext(ctx context.Context) error {
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
	var errs []error
	for _, status := range m.Statuses() {
		if client, ok := m.Client(status.ID); ok {
			if err := client.StopContext(ctx); err != nil {
				errs = append(errs, fmt.Errorf("tunnel %s: %w", status.ID, err))
			}
		}
	}
	return errors.Join(errs...)
}

// Reconcile preserves unchanged clients and changes only the affected connections.
func (m *Manager) Reconcile(ctx context.Context, cfg CollectionConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	previous := m.clients
	previousCfg := m.configs
	running := m.running
	next := make(map[string]*Client, len(cfg.Instances))
	nextCfg := make(map[string]InstanceConfig, len(cfg.Instances))
	var stop []*Client
	var start []*Client
	for _, instance := range cfg.Instances {
		nextCfg[instance.ID] = instance
		if old, ok := previous[instance.ID]; ok && previousCfg[instance.ID] == instance {
			next[instance.ID] = old
			continue
		}
		if old, ok := previous[instance.ID]; ok {
			stop = append(stop, old)
		}
		client := NewConfiguredWithLogger(instance.clientConfig(), m.runtime, m.logger)
		next[instance.ID] = client
		if running && instance.Enabled {
			start = append(start, client)
		}
	}
	for id, old := range previous {
		if _, ok := next[id]; !ok {
			stop = append(stop, old)
		}
	}
	m.clients, m.configs = next, nextCfg
	m.mu.Unlock()
	var errs []error
	for _, client := range stop {
		if err := client.StopContext(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	for _, client := range start {
		if err := client.StartContext(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
