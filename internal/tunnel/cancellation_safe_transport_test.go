package tunnel

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/tunnel-client/pkg/mcpclient"
)

type trackingConnection struct {
	readErr error
	closed  int
}

func (c *trackingConnection) Read(context.Context) (jsonrpc.Message, error) { return nil, c.readErr }
func (c *trackingConnection) Write(context.Context, jsonrpc.Message) error  { return nil }
func (c *trackingConnection) Close() error {
	c.closed++
	return nil
}
func (c *trackingConnection) SessionID() string { return "" }

var _ sdkmcp.Connection = (*trackingConnection)(nil)

func TestCancellationSafeTransportKeepsPipeOpenAfterRequestCancellation(t *testing.T) {
	serverTransport, tunnelTransport := newCancellationSafeInMemoryTransports()
	serverConn, err := serverTransport.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer serverConn.Close()
	forwarding := mcpclient.NewForwardingTransport(mcpclient.NewSharedConnectionTransport(withSessionTransport(tunnelTransport)))
	tunnelConn, err := forwarding.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tunnelConn.Read(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read error = %v, want context canceled", err)
	}

	nextConn, err := forwarding.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer nextConn.Close()
	notification := &jsonrpc.Request{Method: "test/ping"}
	writeDone := make(chan error, 1)
	go func() { writeDone <- serverConn.Write(context.Background(), notification) }()
	readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
	defer readCancel()
	message, err := nextConn.Read(readCtx)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := message.(*jsonrpc.Request)
	if !ok || got.Method != notification.Method || got.ID.IsValid() {
		t.Fatalf("message = %#v, want notification %q", message, notification.Method)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
}

func TestCancellationSafeConnectionStillClosesAfterRealTransportError(t *testing.T) {
	base := &trackingConnection{readErr: fmt.Errorf("broken transport")}
	conn := &cancellationSafeConnection{base: base}
	if _, err := conn.Read(context.Background()); err == nil {
		t.Fatal("Read unexpectedly succeeded")
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if base.closed != 1 {
		t.Fatalf("base close count = %d, want 1", base.closed)
	}
}

func TestCancellationSafeTransportKeepsPipeOpenWhenContextCancelsAfterWrite(t *testing.T) {
	serverTransport, tunnelTransport := newCancellationSafeInMemoryTransports()
	serverConn, err := serverTransport.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer serverConn.Close()
	forwarding := mcpclient.NewForwardingTransport(mcpclient.NewSharedConnectionTransport(withSessionTransport(tunnelTransport)))
	ctx, cancel := context.WithCancel(context.Background())
	conn, err := forwarding.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}

	first := &jsonrpc.Request{Method: "test/first"}
	readFirst := make(chan error, 1)
	go func() {
		message, err := serverConn.Read(context.Background())
		if err == nil {
			request, ok := message.(*jsonrpc.Request)
			if !ok || request.Method != first.Method {
				err = fmt.Errorf("first message = %#v", message)
			}
		}
		readFirst <- err
	}()
	if _, err := conn.Write(ctx, nil, first); err != nil {
		t.Fatal(err)
	}
	if err := <-readFirst; err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	next, err := forwarding.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	second := &jsonrpc.Request{Method: "test/second"}
	writeDone := make(chan error, 1)
	go func() { writeDone <- serverConn.Write(context.Background(), second) }()
	readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
	defer readCancel()
	message, err := next.Read(readCtx)
	if err != nil {
		t.Fatal(err)
	}
	request, ok := message.(*jsonrpc.Request)
	if !ok || request.Method != second.Method {
		t.Fatalf("second message = %#v", message)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
}
