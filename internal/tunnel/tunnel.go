package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	tunnelclient "github.com/openai/tunnel-client"
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

type serverRun struct {
	done chan struct{}
	err  error
}

type Client struct {
	mu         sync.RWMutex
	config     Config
	runtime    *tools.Runtime
	factory    backendFactory
	backend    backend
	cancel     context.CancelFunc
	serverRun  *serverRun
	readyCh    chan struct{}
	doneCh     chan struct{}
	running    bool
	ready      bool
	stopping   bool
	generation uint64
	startedAt  time.Time
	lastError  string
}

func New(id, key string, runtime *tools.Runtime) *Client {
	return NewConfigured(Config{ID: id, APIKey: key}, runtime)
}

func NewConfigured(cfg Config, runtime *tools.Runtime) *Client {
	return newConfigured(cfg, runtime, newOpenAIBackend)
}

func newConfigured(cfg Config, runtime *tools.Runtime, factory backendFactory) *Client {
	if factory == nil {
		factory = newOpenAIBackend
	}
	return &Client{config: cfg, runtime: runtime, factory: factory}
}

func newOpenAIBackend(cfg Config, transport sdkmcp.Transport) (backend, error) {
	return tunnelclient.New(tunnelclient.Config{
		TunnelID:            cfg.ID,
		APIKey:              cfg.APIKey,
		ControlPlaneBaseURL: cfg.ControlPlaneBaseURL,
		OrganizationID:      cfg.OrganizationID,
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
	if !c.config.Enabled {
		c.mu.Unlock()
		return nil
	}
	if c.running || c.stopping {
		c.mu.Unlock()
		return errors.New("OpenAI tunnel already running")
	}
	if c.runtime == nil || c.runtime.Registry == nil {
		c.mu.Unlock()
		return errors.New("OpenAI tunnel requires an MCP tools runtime")
	}
	if err := ValidateConfig(c.config); err != nil {
		c.lastError = err.Error()
		c.mu.Unlock()
		return err
	}

	bridge, err := newSDKBridge(c.runtime)
	if err != nil {
		c.lastError = err.Error()
		c.mu.Unlock()
		return err
	}
	serverTransport, tunnelTransport := sdkmcp.NewInMemoryTransports()
	tunnelBackend, err := c.factory(c.config, tunnelTransport)
	if err != nil {
		c.lastError = err.Error()
		c.mu.Unlock()
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

	go c.watchReady(generation, tunnelBackend, runCtx)
	go c.watchContext(generation, runCtx)
	go c.watchServer(generation, run)
	return nil
}

func (c *Client) watchReady(generation uint64, tunnelBackend backend, ctx context.Context) {
	err := tunnelBackend.WaitUntilReady(ctx)
	if err != nil {
		if ctx.Err() == nil {
			c.mu.Lock()
			if c.generation == generation && c.running && !c.stopping {
				c.lastError = err.Error()
			}
			c.mu.Unlock()
		}
		return
	}
	c.mu.Lock()
	if c.generation == generation && c.running && !c.ready {
		c.ready = true
		close(c.readyCh)
	}
	c.mu.Unlock()
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
	if active {
		c.lastError = "embedded MCP server stopped: " + run.err.Error()
	}
	c.mu.Unlock()
	if !active {
		return
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), defaultStopTimeout)
	defer cancel()
	_ = c.stopGeneration(stopCtx, generation)
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
	}
	c.mu.Unlock()
	return errors.Join(backendErr, serverErr)
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
