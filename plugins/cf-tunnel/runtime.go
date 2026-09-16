package cftunnel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/redact"
	"go.mewis.me/chatgpt-mcp/plugins/cf-tunnel/internal/cloudflared"
)

type StartFunc func(context.Context, cloudflared.Config) (*cloudflared.Tunnel, error)

type TargetStatus struct {
	Target     string
	Desired    bool
	Running    bool
	Ready      bool
	Restarting bool
	URL        string
	Origin     string
	LastError  string
}

type Status struct {
	Enabled bool
	Targets []TargetStatus
}

type slot struct {
	gen        int
	cancel     context.CancelFunc
	parent     context.Context
	desired    bool
	origin     string
	url        string
	lastError  string
	running    bool
	ready      bool
	restarting bool
}

type Manager struct {
	mu             sync.Mutex
	start          StartFunc
	observer       LifecycleObserver
	pending        []LifecycleEvent
	enabled        bool
	reconnectDelay time.Duration
	slots          map[string]*slot
}

var (
	liveMu sync.Mutex
	live   = NewManager()
)

func NewManager() *Manager {
	return &Manager{start: cloudflared.Start, slots: map[string]*slot{}, reconnectDelay: 500 * time.Millisecond}
}

func Sync(ctx context.Context, snap Snapshot) { live.Sync(ctx, snap) }
func Stop()                                   { live.Stop() }
func LiveStatus() Status                      { return live.Status() }

func SetLiveStart(fn StartFunc) StartFunc {
	liveMu.Lock()
	defer liveMu.Unlock()
	return live.SwapStart(fn)
}

func SetLiveObserver(fn LifecycleObserver) {
	liveMu.Lock()
	defer liveMu.Unlock()
	live.SetObserver(fn)
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

func (m *Manager) SetObserver(fn LifecycleObserver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observer = fn
}

func (m *Manager) Sync(ctx context.Context, snap Snapshot) {
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	m.enabled = snap.PluginEnabled
	m.syncTargetLocked(ctx, TargetMCP, snap.PluginEnabled, snap.DesiredMCP, snap.MCP)
	m.syncTargetLocked(ctx, TargetAdmin, snap.PluginEnabled, snap.DesiredAdmin, snap.Admin)
	events, obs := m.takePendingLocked()
	m.mu.Unlock()
	notifyObserver(obs, events)
}

func (m *Manager) Stop() {
	m.mu.Lock()
	m.enabled = false
	m.stopLocked(TargetMCP)
	m.stopLocked(TargetAdmin)
	events, obs := m.takePendingLocked()
	m.mu.Unlock()
	notifyObserver(obs, events)
}

func (m *Manager) ApplyStart(ctx context.Context, target, origin string) error {
	parsed, err := ParseTarget(target)
	if err != nil || parsed == "all" {
		if err != nil {
			return err
		}
		return fmt.Errorf("start requires target mcp or admin")
	}
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return errors.New("origin is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	m.enabled = true
	slot := m.ensureLocked(parsed)
	slot.desired = true
	if slot.running && slot.origin == origin && slot.lastError == "" {
		m.mu.Unlock()
		return nil
	}
	m.stopLocked(parsed)
	slot = m.ensureLocked(parsed)
	slot.desired = true
	m.startLocked(ctx, parsed, origin, false)
	events, obs := m.takePendingLocked()
	m.mu.Unlock()
	notifyObserver(obs, events)
	return nil
}

func (m *Manager) ApplyStop(target string) error {
	parsed, err := ParseTarget(target)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if parsed == "all" {
		m.enabled = false
		m.stopLocked(TargetMCP)
		m.stopLocked(TargetAdmin)
	} else {
		m.stopLocked(parsed)
	}
	events, obs := m.takePendingLocked()
	m.mu.Unlock()
	notifyObserver(obs, events)
	return nil
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
		slot.lastError = sanitizeError(endpoint.AuthErr)
		m.pending = append(m.pending, LifecycleEvent{State: LifecycleDegraded, Target: target, Error: slot.lastError})
		return
	}
	if !endpoint.Ready {
		m.stopLocked(target)
		slot.desired = true
		slot.lastError = ErrListenerNotReady.Error()
		m.pending = append(m.pending, LifecycleEvent{State: LifecycleDegraded, Target: target, Error: slot.lastError})
		return
	}
	origin := loopbackOrigin(endpoint.Port)
	if slot.running && slot.origin == origin && slot.lastError == "" {
		return
	}
	m.stopLocked(target)
	slot.desired = true
	m.startLocked(ctx, target, origin, false)
}

func (m *Manager) startLocked(ctx context.Context, target, origin string, reconnect bool) {
	slot := m.ensureLocked(target)
	if slot.cancel != nil {
		slot.cancel()
	}
	runCtx, cancel := context.WithCancel(ctx)
	slot.gen++
	gen := slot.gen
	slot.cancel = cancel
	slot.parent = ctx
	slot.origin = origin
	slot.url = ""
	if !reconnect {
		slot.lastError = ""
	}
	slot.running = true
	slot.ready = false
	slot.restarting = reconnect
	start := m.start
	if reconnect {
		m.pending = append(m.pending, LifecycleEvent{State: LifecycleReconnecting, Target: target, Origin: origin, Error: slot.lastError})
	} else {
		m.pending = append(m.pending, LifecycleEvent{State: LifecycleConnecting, Target: target, Origin: origin})
	}
	go func() {
		tun, err := start(runCtx, cloudflared.Config{OriginURL: origin})
		m.mu.Lock()
		current := m.slots[target]
		if current == nil || current.gen != gen {
			m.mu.Unlock()
			if tun != nil {
				go func() { _ = tun.Wait() }()
			}
			return
		}
		if err != nil {
			current.running = false
			current.ready = false
			current.restarting = false
			current.lastError = sanitizeError(err)
			m.pending = append(m.pending, LifecycleEvent{State: LifecycleDegraded, Target: target, Origin: origin, Error: current.lastError})
			events, obs := m.takePendingLocked()
			m.mu.Unlock()
			notifyObserver(obs, events)
			return
		}
		current.url = tun.URL
		current.ready = true
		current.restarting = false
		current.lastError = ""
		m.pending = append(m.pending, LifecycleEvent{State: LifecycleReady, Target: target, Origin: origin, URL: tun.URL})
		events, obs := m.takePendingLocked()
		m.mu.Unlock()
		notifyObserver(obs, events)
		m.watch(target, origin, gen, runCtx, tun)
	}()
}

func (m *Manager) watch(target, origin string, gen int, runCtx context.Context, tun *cloudflared.Tunnel) {
	waitErr := tun.Wait()
	m.mu.Lock()
	current := m.slots[target]
	if current == nil || current.gen != gen {
		m.mu.Unlock()
		return
	}
	canceled := runCtx.Err() != nil || errors.Is(waitErr, context.Canceled)
	if canceled || !current.desired {
		current.running = false
		current.ready = false
		current.restarting = false
		current.url = ""
		if !canceled && waitErr != nil {
			current.lastError = sanitizeError(waitErr)
		}
		m.pending = append(m.pending, LifecycleEvent{State: LifecycleStopped, Target: target, Origin: origin})
		events, obs := m.takePendingLocked()
		m.mu.Unlock()
		notifyObserver(obs, events)
		return
	}
	current.ready = false
	current.restarting = true
	current.running = true
	current.url = ""
	current.lastError = sanitizeError(waitErr)
	parent := current.parent
	delay := m.reconnectDelay
	m.pending = append(m.pending, LifecycleEvent{State: LifecycleDegraded, Target: target, Origin: origin, Error: current.lastError})
	events, obs := m.takePendingLocked()
	m.mu.Unlock()
	notifyObserver(obs, events)
	if parent == nil {
		parent = context.Background()
	}
	if delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-parent.Done():
			timer.Stop()
			return
		}
	}
	m.mu.Lock()
	current = m.slots[target]
	if current == nil || current.gen != gen || !current.desired {
		m.mu.Unlock()
		return
	}
	m.startLocked(parent, target, current.origin, true)
	events, obs = m.takePendingLocked()
	m.mu.Unlock()
	notifyObserver(obs, events)
}

func (m *Manager) stopLocked(target string) {
	slot := m.slots[target]
	if slot == nil {
		return
	}
	active := slot.running || slot.ready || slot.restarting
	if slot.cancel != nil {
		slot.cancel()
		slot.cancel = nil
	}
	slot.gen++
	slot.running = false
	slot.ready = false
	slot.restarting = false
	slot.url = ""
	slot.origin = ""
	if active {
		m.pending = append(m.pending, LifecycleEvent{State: LifecycleStopped, Target: target})
	}
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
		Target:     target,
		Desired:    slot.desired,
		Running:    slot.running,
		Ready:      slot.ready,
		Restarting: slot.restarting,
		URL:        slot.url,
		Origin:     slot.origin,
		LastError:  slot.lastError,
	}
}

func (m *Manager) takePendingLocked() ([]LifecycleEvent, LifecycleObserver) {
	events := m.pending
	m.pending = nil
	return events, m.observer
}

func notifyObserver(obs LifecycleObserver, events []LifecycleEvent) {
	if obs == nil {
		return
	}
	for _, event := range events {
		obs(event)
	}
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return redact.Text(err.Error())
}
