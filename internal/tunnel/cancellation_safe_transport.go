package tunnel

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type cancellationSafeTransport struct {
	base sdkmcp.Transport
	mu   sync.Mutex
	conn sdkmcp.Connection
}

type cancellationSafeConnection struct {
	base          sdkmcp.Connection
	lifecycleCtx  context.Context
	preserveClose atomic.Bool
}

func newCancellationSafeInMemoryTransports() (sdkmcp.Transport, sdkmcp.Transport) {
	serverConn, tunnelConn := net.Pipe()
	server := &sdkmcp.IOTransport{Reader: serverConn, Writer: serverConn}
	tunnel := &cancellationSafeTransport{base: &sdkmcp.IOTransport{Reader: tunnelConn, Writer: tunnelConn}}
	return server, tunnel
}

func (t *cancellationSafeTransport) Connect(ctx context.Context) (sdkmcp.Connection, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		conn, err := t.base.Connect(ctx)
		if err != nil {
			return nil, err
		}
		t.conn = conn
	}
	return &cancellationSafeConnection{base: t.conn, lifecycleCtx: ctx}, nil
}

func (c *cancellationSafeConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	msg, err := c.base.Read(ctx)
	c.recordError(ctx, err)
	return msg, err
}

func (c *cancellationSafeConnection) Write(ctx context.Context, msg jsonrpc.Message) error {
	err := c.base.Write(ctx, msg)
	c.recordError(ctx, err)
	return err
}

func (c *cancellationSafeConnection) Close() error {
	if c.preserveClose.Swap(false) || contextCancelled(c.lifecycleCtx) {
		return nil
	}
	return c.base.Close()
}

func (c *cancellationSafeConnection) SessionID() string { return c.base.SessionID() }

func (c *cancellationSafeConnection) recordError(ctx context.Context, err error) {
	preserve := err != nil && ctx != nil && ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
	c.preserveClose.Store(preserve)
}

func contextCancelled(ctx context.Context) bool { return ctx != nil && ctx.Err() != nil }
