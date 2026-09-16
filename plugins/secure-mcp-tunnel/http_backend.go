package securemcptunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	tunnelclient "github.com/openai/tunnel-client"
	"github.com/openai/tunnel-client/pkg/app"
	"github.com/openai/tunnel-client/pkg/config"
	"github.com/openai/tunnel-client/pkg/controlplane"
	"github.com/openai/tunnel-client/pkg/types"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

const (
	defaultMaxInFlightRequests   = 20
	defaultMCPConnectionMaxTTL   = 10 * time.Minute
	defaultMCPConcurrentRequests = 10
)

type runtimeClient struct {
	app *fx.App

	ready     chan struct{}
	readyOnce sync.Once

	mu      sync.Mutex
	started bool
	closed  bool
}

func newHTTPStreamableBackend(cfg tunnel.Config, log *logger.Logger, transport *sdkmcp.StreamableClientTransport) (tunnel.Backend, error) {
	internalCfg, err := httpStreamableConfig(cfg, transport)
	if err != nil {
		return nil, err
	}
	client := &runtimeClient{ready: make(chan struct{})}
	writer := io.Writer(os.Stderr)
	if log != nil {
		writer = log.LineWriter("TUNNEL")
	}
	client.app = app.NewWithRuntime(
		internalCfg,
		app.RuntimeOptions{DisableHealthAdmin: true},
		fx.Provide(func() io.Writer { return writer }),
		fx.Decorate(func(fetcher controlplane.Fetcher) controlplane.Fetcher {
			return &readyFetcher{delegate: fetcher, ready: client.markReady}
		}),
		fx.WithLogger(func(*slog.Logger) fxevent.Logger { return fxevent.NopLogger }),
	)
	if err := client.app.Err(); err != nil {
		return nil, fmt.Errorf("secure-mcp-tunnel: construct runtime: %w", err)
	}
	return client, nil
}

func httpStreamableConfig(cfg tunnel.Config, transport *sdkmcp.StreamableClientTransport) (*config.Config, error) {
	if transport == nil || strings.TrimSpace(transport.Endpoint) == "" {
		return nil, errors.New("secure-mcp-tunnel: MCP bridge URL is required")
	}
	serverURL, err := url.Parse(strings.TrimSpace(transport.Endpoint))
	if err != nil || serverURL.Scheme == "" || serverURL.Host == "" {
		return nil, errors.New("secure-mcp-tunnel: invalid MCP bridge URL")
	}
	internalCfg, err := embedControlPlaneConfig(cfg)
	if err != nil {
		return nil, err
	}
	internalCfg.MCP = config.MCPConfig{
		ServerURL:             serverURL,
		TransportKind:         config.MCPTransportHTTPStreamable,
		ConnectionMaxTTL:      defaultMCPConnectionMaxTTL,
		MaxConcurrentRequests: defaultMCPConcurrentRequests,
		ChannelBindings: []config.MCPChannelBinding{{
			Channel:       types.DefaultChannel,
			TransportKind: config.MCPTransportHTTPStreamable,
			ServerURL:     serverURL,
		}},
	}
	return internalCfg, nil
}

func embedControlPlaneConfig(cfg tunnel.Config) (*config.Config, error) {
	tunnelID := strings.TrimSpace(cfg.ID)
	if err := config.ValidateTunnelID(tunnelID); err != nil {
		return nil, fmt.Errorf("secure-mcp-tunnel: %w", err)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("secure-mcp-tunnel: API key is required")
	}
	baseURLRaw := strings.TrimSpace(cfg.ControlPlaneBaseURL)
	if baseURLRaw == "" {
		baseURLRaw = tunnelclient.DefaultControlPlaneBaseURL
	}
	baseURL, err := url.Parse(baseURLRaw)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, errors.New("secure-mcp-tunnel: invalid control-plane base URL")
	}
	return &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			BaseURL:             baseURL,
			TunnelID:            types.TunnelID(tunnelID),
			OrganizationID:      strings.TrimSpace(cfg.OrganizationID),
			APIKey:              cfg.APIKey,
			MaxInFlightRequests: defaultMaxInFlightRequests,
			PollTimeout:         2 * time.Second,
		},
		Logging: config.LoggingConfig{
			Level:  slog.LevelInfo,
			Format: config.LogFormatStructText,
		},
	}, nil
}

func (c *runtimeClient) Start(ctx context.Context) error {
	if c == nil || c.app == nil {
		return errors.New("secure-mcp-tunnel: client is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return tunnelclient.ErrClosed
	}
	if c.started {
		return nil
	}
	if err := c.app.Start(ctx); err != nil {
		return fmt.Errorf("secure-mcp-tunnel: start runtime: %w", err)
	}
	c.started = true
	return nil
}

func (c *runtimeClient) WaitUntilReady(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-c.Ready():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *runtimeClient) Ready() <-chan struct{} {
	if c == nil || c.ready == nil {
		return make(chan struct{})
	}
	return c.ready
}

func (c *runtimeClient) Done() <-chan os.Signal {
	if c == nil || c.app == nil {
		return nil
	}
	return c.app.Done()
}

func (c *runtimeClient) Stop(ctx context.Context) error {
	if c == nil || c.app == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	if !c.started {
		c.closed = true
		return nil
	}
	if err := c.app.Stop(ctx); err != nil {
		return fmt.Errorf("secure-mcp-tunnel: stop runtime: %w", err)
	}
	c.started = false
	c.closed = true
	return nil
}

func (c *runtimeClient) markReady() {
	if c == nil {
		return
	}
	c.readyOnce.Do(func() { close(c.ready) })
}

type readyFetcher struct {
	delegate controlplane.Fetcher
	ready    func()
}

func (f *readyFetcher) Poll(ctx context.Context, limit int) ([]controlplane.PolledCommand, types.TunnelServiceRequestID, error) {
	commands, requestID, err := f.delegate.Poll(ctx, limit)
	if err == nil && f.ready != nil {
		f.ready()
	}
	return commands, requestID, err
}
