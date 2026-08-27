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
	cancel    context.CancelFunc
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
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.config.Enabled {
		return nil
	}
	if c.cmd != nil {
		return errors.New("tunnel already running")
	}
	if c.config.Command == "" {
		return errors.New("tunnel command is required")
	}
	processCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(processCtx, c.config.Command, c.config.Args...)
	cmd.Env = append(os.Environ(),
		"CHATGPT_MCP_TUNNEL_ID="+c.config.ID,
		"CHATGPT_MCP_TUNNEL_API_KEY="+c.config.APIKey,
		"CHATGPT_MCP_TUNNEL_ORIGIN="+c.config.Origin,
		"CHATGPT_MCP_TUNNEL_PUBLIC_URL="+c.config.PublicURL,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		c.lastError = err.Error()
		return err
	}
	c.cmd = cmd
	c.cancel = cancel
	c.startedAt = time.Now()
	c.lastError = ""
	go c.wait(cmd, processCtx)
	return nil
}

func (c *Client) wait(cmd *exec.Cmd, ctx context.Context) {
	err := cmd.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd != cmd {
		return
	}
	if err != nil && ctx.Err() == nil {
		c.lastError = err.Error()
	}
	c.cmd = nil
	c.cancel = nil
	c.startedAt = time.Time{}
}

func (c *Client) Stop() error {
	c.mu.Lock()
	cmd := c.cmd
	cancel := c.cancel
	c.cmd = nil
	c.cancel = nil
	c.startedAt = time.Time{}
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
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
	return map[string]string{"CHATGPT_MCP_TUNNEL_ID": c.config.ID, "CHATGPT_MCP_TUNNEL_API_KEY": c.config.APIKey, "CHATGPT_MCP_TUNNEL_ORIGIN": c.config.Origin, "CHATGPT_MCP_TUNNEL_PUBLIC_URL": c.config.PublicURL, "CHATGPT_MCP_TUNNEL_ENABLED": strconv.FormatBool(c.config.Enabled)}
}
