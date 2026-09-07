package tunnel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/tunnel-client/pkg/mcpclient"
	"github.com/openai/tunnel-client/pkg/tunnelctx"
	"github.com/openai/tunnel-client/pkg/types"
)

type trackingTransport struct{ conn sdkmcp.Connection }
type trackingConnection struct {
	readErr error
	closed  int
}

func (t *trackingTransport) Connect(context.Context) (sdkmcp.Connection, error) { return t.conn, nil }
func (c *trackingConnection) Read(context.Context) (jsonrpc.Message, error)     { return nil, c.readErr }
func (c *trackingConnection) Write(context.Context, jsonrpc.Message) error      { return nil }
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
	transport := &cancellationSafeTransport{base: &trackingTransport{conn: base}}
	conn, err := transport.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	readCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := conn.Read(readCtx); err == nil {
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
	callerID, _ := jsonrpc.MakeID("first")
	ctx, cancel := context.WithCancel(tunnelTestRequestContext(context.Background(), "first", callerID))
	conn, err := forwarding.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}

	first := &jsonrpc.Request{ID: callerID, Method: "test/first"}
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
	cancelMessage := readServerMessage(t, serverConn)
	if request, ok := cancelMessage.(*jsonrpc.Request); !ok || request.Method != "notifications/cancelled" {
		t.Fatalf("cancel message = %#v", cancelMessage)
	}
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

func TestCancellationSafeTransportRoutesConcurrentResponsesByLogicalRequest(t *testing.T) {
	serverTransport, tunnelTransport := newCancellationSafeInMemoryTransports()
	serverConn, err := serverTransport.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer serverConn.Close()
	forwarding := mcpclient.NewForwardingTransport(mcpclient.NewSharedConnectionTransport(withSessionTransport(tunnelTransport)))
	originalID, _ := jsonrpc.MakeID("same-id")
	ctxA := tunnelTestRequestContext(context.Background(), "command-a", originalID)
	ctxB := tunnelTestRequestContext(context.Background(), "command-b", originalID)
	connA, err := forwarding.Connect(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	connB, err := forwarding.Connect(ctxB)
	if err != nil {
		t.Fatal(err)
	}
	wireA := writeForwardingRequestAndReadServer(t, connA, ctxA, serverConn, &jsonrpc.Request{ID: originalID, Method: "test/a"})
	wireB := writeForwardingRequestAndReadServer(t, connB, ctxB, serverConn, &jsonrpc.Request{ID: originalID, Method: "test/b"})
	if wireA.ID == originalID || wireB.ID == originalID || wireA.ID == wireB.ID {
		t.Fatalf("wire ids a=%v b=%v original=%v", wireA.ID.Raw(), wireB.ID.Raw(), originalID.Raw())
	}
	writeServerMessage(t, serverConn, &jsonrpc.Response{ID: wireB.ID, Result: json.RawMessage(`{"which":"b"}`)})
	writeServerMessage(t, serverConn, &jsonrpc.Response{ID: wireA.ID, Result: json.RawMessage(`{"which":"a"}`)})
	responseA := readForwardingResponse(t, connA, ctxA)
	responseB := readForwardingResponse(t, connB, ctxB)
	if responseA.ID != originalID || responseB.ID != originalID || string(responseA.Result) != `{"which":"a"}` || string(responseB.Result) != `{"which":"b"}` {
		t.Fatalf("responses a=%#v b=%#v", responseA, responseB)
	}
}

func TestCancellationSafeTransportDropsLateCancelledResponse(t *testing.T) {
	serverTransport, tunnelTransport := newCancellationSafeInMemoryTransports()
	serverConn, err := serverTransport.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer serverConn.Close()
	forwarding := mcpclient.NewForwardingTransport(mcpclient.NewSharedConnectionTransport(withSessionTransport(tunnelTransport)))
	originalID, _ := jsonrpc.MakeID("reused-id")
	baseA := tunnelTestRequestContext(context.Background(), "command-cancelled", originalID)
	ctxA, cancelA := context.WithCancel(baseA)
	connA, err := forwarding.Connect(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	wireA := writeForwardingRequestAndReadServer(t, connA, ctxA, serverConn, &jsonrpc.Request{ID: originalID, Method: "test/cancelled"})
	cancelA()
	cancelRequest := readServerRequest(t, serverConn, "notifications/cancelled")
	var params sdkmcp.CancelledParams
	if err := json.Unmarshal(cancelRequest.Params, &params); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(params.RequestID) != fmt.Sprint(wireA.ID.Raw()) {
		t.Fatalf("cancel request id = %v, want %v", params.RequestID, wireA.ID.Raw())
	}
	if err := connA.Close(); err != nil {
		t.Fatal(err)
	}

	ctxB := tunnelTestRequestContext(context.Background(), "command-next", originalID)
	connB, err := forwarding.Connect(ctxB)
	if err != nil {
		t.Fatal(err)
	}
	wireB := writeForwardingRequestAndReadServer(t, connB, ctxB, serverConn, &jsonrpc.Request{ID: originalID, Method: "test/next"})
	if wireB.ID == wireA.ID {
		t.Fatalf("reused canceled wire id %v", wireB.ID.Raw())
	}
	writeServerMessage(t, serverConn, &jsonrpc.Response{ID: wireA.ID, Result: json.RawMessage(`{"late":true}`)})
	writeServerMessage(t, serverConn, &jsonrpc.Response{ID: wireB.ID, Result: json.RawMessage(`{"late":false}`)})
	response := readForwardingResponse(t, connB, ctxB)
	if response.ID != originalID || string(response.Result) != `{"late":false}` {
		t.Fatalf("response = %#v", response)
	}
}

func tunnelTestRequestContext(parent context.Context, commandID string, rpcID jsonrpc.ID) context.Context {
	ctx := tunnelctx.ContextWithControlPlaneCommandRequestID(parent, types.ControlPlaneRequestID(commandID))
	return tunnelctx.ContextWithRPCRequestID(ctx, rpcID)
}

func readServerMessage(t *testing.T, conn sdkmcp.Connection) jsonrpc.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	message, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func readServerRequest(t *testing.T, conn sdkmcp.Connection, method string) *jsonrpc.Request {
	t.Helper()
	message := readServerMessage(t, conn)
	request, ok := message.(*jsonrpc.Request)
	if !ok || request.Method != method {
		t.Fatalf("server message = %#v, want request %q", message, method)
	}
	return request
}

func writeForwardingRequestAndReadServer(t *testing.T, conn mcpclient.ForwardingConnection, ctx context.Context, server sdkmcp.Connection, request *jsonrpc.Request) *jsonrpc.Request {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := conn.Write(ctx, nil, request)
		done <- err
	}()
	wire := readServerRequest(t, server, request.Method)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("forwarding write timed out")
	}
	return wire
}

func writeServerMessage(t *testing.T, conn sdkmcp.Connection, message jsonrpc.Message) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- conn.Write(context.Background(), message) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server write timed out")
	}
}

func readForwardingResponse(t *testing.T, conn mcpclient.ForwardingConnection, ctx context.Context) *jsonrpc.Response {
	t.Helper()
	readCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	message, err := conn.Read(readCtx)
	if err != nil {
		t.Fatal(err)
	}
	response, ok := message.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("forwarded message = %#v, want response", message)
	}
	return response
}
