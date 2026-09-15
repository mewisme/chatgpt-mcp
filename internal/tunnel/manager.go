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
	opMu     sync.Mutex
	mu       sync.RWMutex
	runtime  *tools.Runtime
	logger   *logger.Logger
	clients  map[string]*Client
	configs  map[string]InstanceConfig
	running  bool
	observer LifecycleObserver
	factory  backendFactory
}

type managerChange struct {
	old, next  *Client
	oldEnabled bool
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

func (m *Manager) SetLifecycleObserver(observer LifecycleObserver) {
	m.mu.Lock()
	m.observer = observer
	for _, client := range m.clients {
		client.SetLifecycleObserver(observer)
	}
	m.mu.Unlock()
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
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()
	var wg sync.WaitGroup
	errCh := make(chan error, len(m.Statuses()))
	for _, status := range m.Statuses() {
		if status.Enabled {
			if client, ok := m.Client(status.ID); ok {
				wg.Add(1)
				go func(id string, client *Client) {
					defer wg.Done()
					if err := client.StartContext(ctx); err != nil {
						errCh <- fmt.Errorf("tunnel %s: %w", id, err)
					}
				}(status.ID, client)
			}
		}
	}
	wg.Wait()
	close(errCh)
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (m *Manager) StopContext(ctx context.Context) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
	var wg sync.WaitGroup
	errCh := make(chan error, len(m.Statuses()))
	for _, status := range m.Statuses() {
		if client, ok := m.Client(status.ID); ok {
			wg.Add(1)
			go func(id string, client *Client) {
				defer wg.Done()
				if err := client.StopContext(ctx); err != nil {
					errCh <- fmt.Errorf("tunnel %s: %w", id, err)
				}
			}(status.ID, client)
		}
	}
	wg.Wait()
	close(errCh)
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// WaitUntilAnyReady succeeds as soon as one enabled tunnel becomes usable.
func (m *Manager) WaitUntilAnyReady(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if m.Ready() {
		return nil
	}
	statuses := m.Statuses()
	clients := make([]*Client, 0, len(statuses))
	for _, status := range statuses {
		if status.Enabled {
			if client, ok := m.Client(status.ID); ok {
				clients = append(clients, client)
			}
		}
	}
	if len(clients) == 0 {
		return errors.New("no enabled tunnels")
	}
	ready := make(chan struct{}, 1)
	for _, client := range clients {
		go func(client *Client) {
			if client.WaitUntilReady(ctx) == nil {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
		}(client)
	}
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Reconcile preserves unchanged clients and changes only the affected connections.
func (m *Manager) Reconcile(ctx context.Context, cfg CollectionConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.RLock()
	previous := m.clients
	previousCfg := m.configs
	running := m.running
	observer := m.observer
	next := make(map[string]*Client, len(cfg.Instances))
	nextCfg := make(map[string]InstanceConfig, len(cfg.Instances))
	var changes []managerChange
	for _, instance := range cfg.Instances {
		nextCfg[instance.ID] = instance
		if old, ok := previous[instance.ID]; ok && sameTransport(previousCfg[instance.ID], instance) {
			next[instance.ID] = old
			continue
		}
		old := previous[instance.ID]
		client := NewConfiguredWithLogger(instance.clientConfig(), m.runtime, m.logger)
		if m.factory != nil {
			client = newConfigured(instance.clientConfig(), m.runtime, m.factory)
		}
		client.SetLifecycleObserver(observer)
		next[instance.ID] = client
		changes = append(changes, managerChange{old: old, next: client, oldEnabled: old != nil && previousCfg[instance.ID].Enabled})
	}
	for id, old := range previous {
		if _, ok := next[id]; !ok {
			changes = append(changes, managerChange{old: old, oldEnabled: previousCfg[id].Enabled})
		}
	}
	m.mu.RUnlock()
	var applied []managerChange
	for _, item := range changes {
		if item.old != nil {
			if err := item.old.StopContext(ctx); err != nil {
				return errors.Join(fmt.Errorf("stop tunnel %s: %w", item.old.Status().ID, err), rollbackChanges(ctx, applied, running))
			}
		}
		applied = append(applied, item)
		if running && item.next != nil && item.next.Status().Enabled {
			if err := item.next.StartContext(ctx); err != nil {
				return errors.Join(fmt.Errorf("start tunnel %s: %w", item.next.Status().ID, err), rollbackChanges(ctx, applied, running))
			}
		}
	}
	m.mu.Lock()
	m.clients, m.configs = next, nextCfg
	m.mu.Unlock()
	return nil
}

func sameTransport(left, right InstanceConfig) bool {
	left.AdminProfileID, right.AdminProfileID = "", ""
	return left == right
}

func rollbackChanges(ctx context.Context, changes []managerChange, running bool) error {
	var errs []error
	for i := len(changes) - 1; i >= 0; i-- {
		item := changes[i]
		if item.next != nil {
			errs = append(errs, item.next.StopContext(ctx))
		}
		if running && item.old != nil && item.oldEnabled {
			errs = append(errs, item.old.StartContext(ctx))
		}
	}
	return errors.Join(errs...)
}
