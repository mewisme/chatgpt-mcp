package tunnel

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

const (
	gracefulStopTimeout = 2 * time.Second
	defaultStopTimeout  = 5 * time.Second
	redactedSecret      = "[redacted]"
)

type Config struct {
	Enabled   bool     `json:"enabled"`
	ID        string   `json:"id,omitempty"`
	APIKey    string   `json:"api_key,omitempty"`
	Command   string   `json:"command,omitempty"`
	Args      []string `json:"args,omitempty"`
	Origin    string   `json:"origin,omitempty"`
	PublicURL string   `json:"public_url,omitempty"`
}

type Status struct {
	Enabled   bool      `json:"enabled"`
	Running   bool      `json:"running"`
	PID       int       `json:"pid,omitempty"`
	Command   string    `json:"command,omitempty"`
	Origin    string    `json:"origin,omitempty"`
	PublicURL string    `json:"public_url,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

type Client struct {
	mu        sync.RWMutex
	config    Config
	cmd       *exec.Cmd
	done      chan struct{}
	stopping  bool
	startedAt time.Time
	lastError string
}

func New(id, key string) *Client       { return NewConfigured(Config{ID: id, APIKey: key}) }
func NewConfigured(cfg Config) *Client { return &Client{config: cfg} }

func (c *Client) Configure(cfg Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd != nil {
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

func (c *Client) StartContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	if !c.config.Enabled {
		c.mu.Unlock()
		return nil
	}
	if c.cmd != nil {
		c.mu.Unlock()
		return errors.New("tunnel already running")
	}
	if c.config.Command == "" {
		c.mu.Unlock()
		return errors.New("tunnel command is required")
	}
	cmd := exec.Command(c.config.Command, c.config.Args...)
	cmd.Env = append(os.Environ(),
		"CHATGPT_MCP_TUNNEL_ID="+c.config.ID,
		"CHATGPT_MCP_TUNNEL_API_KEY="+c.config.APIKey,
		"CHATGPT_MCP_TUNNEL_ORIGIN="+c.config.Origin,
		"CHATGPT_MCP_TUNNEL_PUBLIC_URL="+c.config.PublicURL,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		c.lastError = err.Error()
		c.mu.Unlock()
		return err
	}
	done := make(chan struct{})
	c.cmd = cmd
	c.done = done
	c.stopping = false
	c.startedAt = time.Now()
	c.lastError = ""
	c.mu.Unlock()

	go c.wait(cmd, done)
	go c.watchContext(ctx, cmd, done)
	return nil
}

func (c *Client) wait(cmd *exec.Cmd, done chan struct{}) {
	err := cmd.Wait()
	c.mu.Lock()
	if c.cmd == cmd {
		if err != nil && !c.stopping {
			c.lastError = err.Error()
		}
		c.cmd = nil
		c.done = nil
		c.stopping = false
		c.startedAt = time.Time{}
	}
	c.mu.Unlock()
	close(done)
}

func (c *Client) watchContext(ctx context.Context, cmd *exec.Cmd, done <-chan struct{}) {
	select {
	case <-done:
		return
	case <-ctx.Done():
		stopCtx, cancel := context.WithTimeout(context.Background(), defaultStopTimeout)
		defer cancel()
		_ = c.stopCommand(stopCtx, cmd, done)
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
	cmd := c.cmd
	done := c.done
	c.mu.RUnlock()
	if cmd == nil || done == nil {
		return nil
	}
	return c.stopCommand(ctx, cmd, done)
}

func (c *Client) stopCommand(ctx context.Context, cmd *exec.Cmd, done <-chan struct{}) error {
	c.mu.Lock()
	if c.cmd != cmd {
		c.mu.Unlock()
		return nil
	}
	c.stopping = true
	c.mu.Unlock()

	if cmd.Process == nil {
		return nil
	}
	signalErr := cmd.Process.Signal(os.Interrupt)
	if signalErr != nil && !errors.Is(signalErr, os.ErrProcessDone) {
		if err := killProcess(cmd.Process); err != nil {
			return err
		}
		return waitProcessExit(ctx, done)
	}

	timer := time.NewTimer(gracefulStopTimeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
	case <-ctx.Done():
	}
	if err := killProcess(cmd.Process); err != nil {
		return err
	}
	return waitProcessExit(ctx, done)
}

func killProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func waitProcessExit(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status := Status{Enabled: c.config.Enabled, Running: c.cmd != nil, Command: c.config.Command, Origin: c.config.Origin, PublicURL: c.config.PublicURL, StartedAt: c.startedAt, LastError: c.lastError}
	if c.cmd != nil && c.cmd.Process != nil {
		status.PID = c.cmd.Process.Pid
	}
	return status
}

func (c *Client) Environment() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	apiKey := ""
	if c.config.APIKey != "" {
		apiKey = redactedSecret
	}
	return map[string]string{
		"CHATGPT_MCP_TUNNEL_ID":         c.config.ID,
		"CHATGPT_MCP_TUNNEL_API_KEY":    apiKey,
		"CHATGPT_MCP_TUNNEL_ORIGIN":     c.config.Origin,
		"CHATGPT_MCP_TUNNEL_PUBLIC_URL": c.config.PublicURL,
		"CHATGPT_MCP_TUNNEL_ENABLED":    strconv.FormatBool(c.config.Enabled),
	}
}
