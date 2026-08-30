package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	tunnelclient "github.com/openai/tunnel-client"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

const (
	ProviderOpenAI     = "openai"
	defaultStopTimeout = 5 * time.Second
)

type Config struct {
	Enabled             bool   `json:"enabled"`
	ID                  string `json:"id,omitempty"`
	APIKey              string `json:"api_key,omitempty"`
	ControlPlaneBaseURL string `json:"control_plane_base_url,omitempty"`
	OrganizationID      string `json:"organization_id,omitempty"`
}

type Status struct {
	Provider            string    `json:"provider"`
	Enabled             bool      `json:"enabled"`
	Running             bool      `json:"running"`
	Ready               bool      `json:"ready"`
	ID                  string    `json:"id,omitempty"`
	ControlPlaneBaseURL string    `json:"control_plane_base_url,omitempty"`
	OrganizationID      string    `json:"organization_id,omitempty"`
	StartedAt           time.Time `json:"started_at,omitempty"`
	LastError           string    `json:"last_error,omitempty"`
}

type backend interface {
	Start(context.Context) error
	Stop(context.Context) error
	WaitUntilReady(context.Context) error
	Done() <-chan os.Signal
}

type backendFactory func(Config, sdkmcp.Transport) (backend, error)

type LifecycleState string

const (
	LifecycleConnecting LifecycleState = "connecting"
	LifecycleReady      LifecycleState = "ready"
	LifecycleDegraded   LifecycleState = "degraded"
	LifecycleStopped    LifecycleState = "stopped"
)

type LifecycleEvent struct {
	State   LifecycleState
	ID      string
	Message string
}

type LifecycleObserver func(LifecycleEvent)

type serverRun struct {
	done chan struct{}
	err  error
}

type Client struct {
	reconfigureMu sync.Mutex
	lifecycleMu   sync.RWMutex
	mu            sync.RWMutex
	config        Config
	runtime       *tools.Runtime
	factory       backendFactory
	backend       backend
	cancel        context.CancelFunc
	serverRun     *serverRun
	readyCh       chan struct{}
	doneCh        chan struct{}
	running       bool
	ready         bool
	stopping      bool
	generation    uint64
	startedAt     time.Time
	lastError     string
	lifecycle     LifecycleObserver
}

func New(id, key string, runtime *tools.Runtime) *Client {
	return NewConfigured(Config{ID: id, APIKey: key}, runtime)
}

func NewConfigured(cfg Config, runtime *tools.Runtime) *Client {
	return NewConfiguredWithLogger(cfg, runtime, nil)
}

func NewConfiguredWithLogger(cfg Config, runtime *tools.Runtime, log *logger.Logger) *Client {
	return newConfigured(cfg, runtime, newOpenAIBackendFactory(log))
}

func newConfigured(cfg Config, runtime *tools.Runtime, factory backendFactory) *Client {
	if factory == nil {
		factory = newOpenAIBackendFactory(nil)
	}
	return &Client{config: cfg, runtime: runtime, factory: factory}
}

func (c *Client) SetLifecycleObserver(observer LifecycleObserver) {
	if c == nil {
		return
	}
	c.lifecycleMu.Lock()
	c.lifecycle = observer
	c.lifecycleMu.Unlock()
}

func (c *Client) emitLifecycle(state LifecycleState, id, message string) {
	if c == nil {
		return
	}
	c.lifecycleMu.RLock()
	observer := c.lifecycle
	c.lifecycleMu.RUnlock()
	if observer != nil {
		observer(LifecycleEvent{State: state, ID: id, Message: message})
	}
}

func newOpenAIBackendFactory(log *logger.Logger) backendFactory {
	if log == nil {
		log = logger.New(logger.Info)
	}
	return func(cfg Config, transport sdkmcp.Transport) (backend, error) {
		return newOpenAIBackend(cfg, transport, log.LineWriter("TUNNEL"))
	}
}

func newOpenAIBackend(cfg Config, transport sdkmcp.Transport, logWriter io.Writer) (backend, error) {
	return tunnelclient.New(tunnelclient.Config{
		TunnelID:            cfg.ID,
		APIKey:              cfg.APIKey,
		ControlPlaneBaseURL: cfg.ControlPlaneBaseURL,
		OrganizationID:      cfg.OrganizationID,
		LogWriter:           logWriter,
	}, transport)
}

func ValidateConfig(cfg Config) error {
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.ID) == "" {
		return errors.New("OpenAI tunnel is enabled but tunnel id is empty")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return errors.New("OpenAI tunnel is enabled but API key is empty")
	}
	if raw := strings.TrimSpace(cfg.ControlPlaneBaseURL); raw != "" {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("invalid OpenAI tunnel control plane base URL %q", raw)
		}
	}
	return nil
}

func (c *Client) Configure(cfg Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running || c.stopping {
		return errors.New("cannot configure a running tunnel")
	}
	c.config = cfg
	c.lastError = ""
	return nil
}

func (c *Client) Reconfigure(cfg Config, persist func() error) error {
	if persist == nil {
		return errors.New("tunnel persistence callback is required")
	}
	if err := ValidateConfig(cfg); err != nil {
		return err
	}
	c.reconfigureMu.Lock()
	defer c.reconfigureMu.Unlock()

	current := c.Config()
	wasRunning := c.Status().Running
	if err := c.Stop(); err != nil {
		return err
	}
	if err := c.Configure(cfg); err != nil {
		return errors.Join(err, c.rollbackReconfigure(current, wasRunning))
	}
	if cfg.Enabled {
		if err := c.Start(); err != nil {
			return errors.Join(err, c.rollbackReconfigure(current, wasRunning))
		}
	}
	if err := persist(); err != nil {
		return errors.Join(err, c.rollbackReconfigure(current, wasRunning))
	}
	return nil
}

func (c *Client) rollbackReconfigure(cfg Config, restart bool) error {
	var rollbackErr error
	if c.Status().Running {
		rollbackErr = errors.Join(rollbackErr, c.Stop())
	}
	rollbackErr = errors.Join(rollbackErr, c.Configure(cfg))
	if restart {
		rollbackErr = errors.Join(rollbackErr, c.Start())
	}
	return rollbackErr
}

func (c *Client) Config() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

func (c *Client) Start() error { return c.StartContext(context.Background()) }

func (c *Client) StartContext(parent context.Context) error {
	if parent == nil {
		parent = context.Background()
	}

	c.mu.Lock()
	id := c.config.ID
	if !c.config.Enabled {
		c.mu.Unlock()
		return nil
	}
	if c.running || c.stopping {
		c.mu.Unlock()
		return errors.New("OpenAI tunnel already running")
	}
	if c.runtime == nil || c.runtime.Registry == nil {
		err := errors.New("OpenAI tunnel requires an MCP tools runtime")
		c.lastError = err.Error()
		c.mu.Unlock()
		c.emitLifecycle(LifecycleDegraded, id, err.Error())
		return err
	}
	if err := ValidateConfig(c.config); err != nil {
		c.lastError = err.Error()
		c.mu.Unlock()
		c.emitLifecycle(LifecycleDegraded, id, err.Error())
		return err
	}

	bridge, err := newSDKBridge(c.runtime)
	if err != nil {
		c.lastError = err.Error()
		c.mu.Unlock()
		c.emitLifecycle(LifecycleDegraded, id, err.Error())
		return err
	}
	serverTransport, tunnelTransport := sdkmcp.NewInMemoryTransports()
	tunnelBackend, err := c.factory(c.config, tunnelTransport)
	if err != nil {
		c.lastError = err.Error()
		c.mu.Unlock()
		c.emitLifecycle(LifecycleDegraded, id, err.Error())
		return err
	}

	runCtx, cancel := context.WithCancel(parent)
	run := &serverRun{done: make(chan struct{})}
	go func() {
		run.err = bridge.Run(runCtx, serverTransport)
		close(run.done)
	}()

	if err := tunnelBackend.Start(runCtx); err != nil {
		cancel()
		c.lastError = err.Error()
		c.mu.Unlock()
		waitRun(context.Background(), run, time.Second)
		c.emitLifecycle(LifecycleDegraded, id, err.Error())
		return err
	}

	c.generation++
	generation := c.generation
	c.backend = tunnelBackend
	c.cancel = cancel
	c.serverRun = run
	c.readyCh = make(chan struct{})
	c.doneCh = make(chan struct{})
	c.running = true
	c.ready = false
	c.stopping = false
	c.startedAt = time.Now()
	c.lastError = ""
	c.mu.Unlock()
	c.emitLifecycle(LifecycleConnecting, id, "waiting for control-plane readiness")

	go c.watchReady(generation, tunnelBackend, runCtx)
	go c.watchContext(generation, runCtx)
	go c.watchServer(generation, run)
	go c.watchBackend(generation, tunnelBackend, runCtx)
	return nil
}

func (c *Client) watchReady(generation uint64, tunnelBackend backend, ctx context.Context) {
	err := tunnelBackend.WaitUntilReady(ctx)
	if err != nil {
		var id string
		active := false
		if ctx.Err() == nil {
			c.mu.Lock()
			if c.generation == generation && c.running && !c.stopping {
				c.lastError = err.Error()
				id = c.config.ID
				active = true
			}
			c.mu.Unlock()
		}
		if active {
			c.emitLifecycle(LifecycleDegraded, id, err.Error())
		}
		return
	}
	var id string
	becameReady := false
	c.mu.Lock()
	if c.generation == generation && c.running && !c.ready {
		c.ready = true
		id = c.config.ID
		becameReady = true
		close(c.readyCh)
	}
	c.mu.Unlock()
	if becameReady {
		c.emitLifecycle(LifecycleReady, id, "control plane ready")
	}
}

func (c *Client) watchContext(generation uint64, ctx context.Context) {
	<-ctx.Done()
	stopCtx, cancel := context.WithTimeout(context.Background(), defaultStopTimeout)
	defer cancel()
	_ = c.stopGeneration(stopCtx, generation)
}

func (c *Client) watchServer(generation uint64, run *serverRun) {
	<-run.done
	if run.err == nil || errors.Is(run.err, context.Canceled) {
		return
	}
	c.mu.Lock()
	active := c.generation == generation && c.running && !c.stopping
	id := ""
	message := ""
	if active {
		message = "embedded MCP server stopped: " + run.err.Error()
		c.lastError = message
		id = c.config.ID
	}
	c.mu.Unlock()
	if !active {
		return
	}
	c.emitLifecycle(LifecycleDegraded, id, message)
	stopCtx, cancel := context.WithTimeout(context.Background(), defaultStopTimeout)
	defer cancel()
	_ = c.stopGeneration(stopCtx, generation)
}

func (c *Client) watchBackend(generation uint64, tunnelBackend backend, ctx context.Context) {
	done := tunnelBackend.Done()
	if done == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case signal, ok := <-done:
		if !ok {
			return
		}
		message := "tunnel runtime stopped"
		if signal != nil {
			message += ": " + signal.String()
		}
		c.mu.Lock()
		active := c.generation == generation && c.running && !c.stopping
		id := ""
		if active {
			c.lastError = message
			id = c.config.ID
		}
		c.mu.Unlock()
		if !active {
			return
		}
		c.emitLifecycle(LifecycleDegraded, id, message)
		stopCtx, cancel := context.WithTimeout(context.Background(), defaultStopTimeout)
		defer cancel()
		_ = c.stopGeneration(stopCtx, generation)
	}
}

func (c *Client) WaitUntilReady(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.RLock()
	if c.ready {
		c.mu.RUnlock()
		return nil
	}
	if !c.running {
		c.mu.RUnlock()
		return errors.New("OpenAI tunnel is not running")
	}
	readyCh := c.readyCh
	doneCh := c.doneCh
	c.mu.RUnlock()

	select {
	case <-readyCh:
		return nil
	case <-doneCh:
		return errors.New("OpenAI tunnel stopped before becoming ready")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultStopTimeout)
	defer cancel()
	return c.StopContext(ctx)
}

func (c *Client) StopContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.RLock()
	generation := c.generation
	c.mu.RUnlock()
	return c.stopGeneration(ctx, generation)
}

func (c *Client) stopGeneration(ctx context.Context, generation uint64) error {
	c.mu.Lock()
	if !c.running || c.generation != generation {
		c.mu.Unlock()
		return nil
	}
	if c.stopping {
		doneCh := c.doneCh
		c.mu.Unlock()
		select {
		case <-doneCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.stopping = true
	id := c.config.ID
	tunnelBackend := c.backend
	cancel := c.cancel
	run := c.serverRun
	doneCh := c.doneCh
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	var backendErr error
	if tunnelBackend != nil {
		backendErr = tunnelBackend.Stop(ctx)
	}
	serverErr := waitRun(ctx, run, 0)
	if errors.Is(serverErr, context.Canceled) {
		serverErr = nil
	}

	c.mu.Lock()
	stopped := false
	if c.generation == generation {
		c.backend = nil
		c.cancel = nil
		c.serverRun = nil
		c.readyCh = nil
		c.doneCh = nil
		c.running = false
		c.ready = false
		c.stopping = false
		c.startedAt = time.Time{}
		close(doneCh)
		stopped = true
	}
	c.mu.Unlock()
	err := errors.Join(backendErr, serverErr)
	if stopped {
		if err != nil {
			c.emitLifecycle(LifecycleDegraded, id, err.Error())
		}
		c.emitLifecycle(LifecycleStopped, id, "tunnel stopped")
	}
	return err
}

func waitRun(ctx context.Context, run *serverRun, fallback time.Duration) error {
	if run == nil {
		return nil
	}
	if fallback > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, fallback)
		defer cancel()
	}
	select {
	case <-run.done:
		return run.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Status{
		Provider: ProviderOpenAI, Enabled: c.config.Enabled, Running: c.running, Ready: c.ready, ID: c.config.ID,
		ControlPlaneBaseURL: c.config.ControlPlaneBaseURL, OrganizationID: c.config.OrganizationID,
		StartedAt: c.startedAt, LastError: c.lastError,
	}
}
