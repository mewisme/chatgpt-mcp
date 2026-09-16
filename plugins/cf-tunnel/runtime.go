package cftunnel

import (
	"context"
	"errors"
	"sync"

	"go.mewis.me/chatgpt-mcp/pkg/cloudflared"
)

type StartFunc func(context.Context, cloudflared.Config) (*cloudflared.Tunnel, error)

type TargetStatus struct {
	Target    string
	Desired   bool
	Running   bool
	Ready     bool
	URL       string
	Origin    string
	LastError string
}

type Status struct {
	Enabled bool
	Targets []TargetStatus
}

type slot struct {
	gen       int
	cancel    context.CancelFunc
	desired   bool
	origin    string
	url       string
	lastError string
	running   bool
	ready     bool
}

type Manager struct {
	mu      sync.Mutex
	start   StartFunc
	enabled bool
	slots   map[string]*slot
}

var (
	liveMu sync.Mutex
	live   = NewManager()
)

func NewManager() *Manager {
	return &Manager{start: cloudflared.Start, slots: map[string]*slot{}}
}

func Sync(ctx context.Context, snap Snapshot) { live.Sync(ctx, snap) }
func Stop()                                   { live.Stop() }
func LiveStatus() Status                      { return live.Status() }

func SetLiveStart(fn StartFunc) StartFunc {
	liveMu.Lock()
	defer liveMu.Unlock()
	return live.SwapStart(fn)
}

func (m *Manager) SwapStart(fn StartFunc) StartFunc {
	m.mu.Lock()
	defer m.mu.Unlock()
	previous := m.start
	if fn == nil {
		m.start = cloudflared.Start
	} else {
		m.start = fn
	}
	return previous
}

func (m *Manager) Sync(ctx context.Context, snap Snapshot) {
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = snap.PluginEnabled
	m.syncTargetLocked(ctx, TargetMCP, snap.PluginEnabled, snap.DesiredMCP, snap.MCP)
	m.syncTargetLocked(ctx, TargetAdmin, snap.PluginEnabled, snap.DesiredAdmin, snap.Admin)
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = false
	m.stopLocked(TargetMCP)
	m.stopLocked(TargetAdmin)
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{Enabled: m.enabled, Targets: []TargetStatus{m.statusLocked(TargetMCP), m.statusLocked(TargetAdmin)}}
}

func (m *Manager) syncTargetLocked(ctx context.Context, target string, pluginEnabled, desired bool, endpoint Endpoint) {
	slot := m.ensureLocked(target)
	slot.desired = pluginEnabled && desired
	if !pluginEnabled || !desired {
		m.stopLocked(target)
		slot.desired = false
		slot.lastError = ""
		return
	}
	if endpoint.AuthErr != nil {
		m.stopLocked(target)
		slot.desired = true
		slot.lastError = endpoint.AuthErr.Error()
		return
	}
	if !endpoint.Ready {
		m.stopLocked(target)
		slot.desired = true
		slot.lastError = ErrListenerNotReady.Error()
		return
	}
	origin := loopbackOrigin(endpoint.Port)
	if slot.running && slot.origin == origin && slot.lastError == "" {
		return
	}
	m.stopLocked(target)
	slot.desired = true
	m.startLocked(ctx, target, origin)
}

func (m *Manager) startLocked(ctx context.Context, target, origin string) {
	slot := m.ensureLocked(target)
	runCtx, cancel := context.WithCancel(ctx)
	slot.gen++
	gen := slot.gen
	slot.cancel = cancel
	slot.origin = origin
	slot.url = ""
	slot.lastError = ""
	slot.running = true
	slot.ready = false
	start := m.start
	go func() {
		tun, err := start(runCtx, cloudflared.Config{OriginURL: origin})
		m.mu.Lock()
		defer m.mu.Unlock()
		current := m.slots[target]
		if current == nil || current.gen != gen {
			if tun != nil {
				go func() { _ = tun.Wait() }()
			}
			return
		}
		if err != nil {
			current.running = false
			current.ready = false
			current.lastError = err.Error()
			return
		}
		current.url = tun.URL
		current.ready = true
		go func() {
			waitErr := tun.Wait()
			m.mu.Lock()
			defer m.mu.Unlock()
			current := m.slots[target]
			if current == nil || current.gen != gen {
				return
			}
			current.running = false
			current.ready = false
			current.url = ""
			if waitErr != nil && !errors.Is(waitErr, context.Canceled) && runCtx.Err() == nil {
				current.lastError = waitErr.Error()
			}
		}()
	}()
}

func (m *Manager) stopLocked(target string) {
	slot := m.slots[target]
	if slot == nil {
		return
	}
	if slot.cancel != nil {
		slot.cancel()
		slot.cancel = nil
	}
	slot.gen++
	slot.running = false
	slot.ready = false
	slot.url = ""
	slot.origin = ""
}

func (m *Manager) ensureLocked(target string) *slot {
	if m.slots[target] == nil {
		m.slots[target] = &slot{}
	}
	return m.slots[target]
}

func (m *Manager) statusLocked(target string) TargetStatus {
	slot := m.slots[target]
	if slot == nil {
		return TargetStatus{Target: target}
	}
	return TargetStatus{
		Target:    target,
		Desired:   slot.desired,
		Running:   slot.running,
		Ready:     slot.ready,
		URL:       slot.url,
		Origin:    slot.origin,
		LastError: slot.lastError,
	}
}
