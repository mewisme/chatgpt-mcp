package securemcptunnel

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/tunnel-client/pkg/mcpclient"

	localmcp "go.mewis.me/chatgpt-mcp/internal/mcp"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func TestTunnelWrappedStreamableHTTPForwardsConcurrentToolCalls(t *testing.T) {
	bridge := startProbeBridge(t)
	forwarding := mcpclient.NewForwardingTransport(mcpclient.NewSharedConnectionTransport(
		wrapPluginMCPTransport(&sdkmcp.StreamableClientTransport{
			Endpoint: bridge.URL, DisableStandaloneSSE: true,
		}, "tunnel_1", "wsl"),
	))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	initID, err := jsonrpc.MakeID("init")
	if err != nil {
		t.Fatal(err)
	}
	initCtx := tunnelTestRequestContext(ctx, "init", initID)
	conn, err := forwarding.Connect(initCtx)
	if err != nil {
		t.Fatal(err)
	}
	initializeTunnelMCP(t, conn, initCtx, initID)

	idA, err := jsonrpc.MakeID("call-a")
	if err != nil {
		t.Fatal(err)
	}
	idB, err := jsonrpc.MakeID("call-b")
	if err != nil {
		t.Fatal(err)
	}
	ctxA := tunnelTestRequestContext(ctx, "cmd-a", idA)
	ctxB := tunnelTestRequestContext(ctx, "cmd-b", idB)
	connA, err := forwarding.Connect(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	connB, err := forwarding.Connect(ctxB)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	errA := make(chan error, 1)
	errB := make(chan error, 1)
	go func() {
		defer wg.Done()
		errA <- callBridgeProbe(connA, ctxA, idA)
	}()
	go func() {
		defer wg.Done()
		errB <- callBridgeProbe(connB, ctxB, idB)
	}()
	wg.Wait()
	if err := <-errA; err != nil {
		t.Fatal(err)
	}
	if err := <-errB; err != nil {
		t.Fatal(err)
	}
}

func TestTunnelWrappedStreamableHTTPSurvivesLogicalClose(t *testing.T) {
	bridge := startProbeBridge(t)
	forwarding := mcpclient.NewForwardingTransport(mcpclient.NewSharedConnectionTransport(
		wrapPluginMCPTransport(&sdkmcp.StreamableClientTransport{
			Endpoint: bridge.URL, DisableStandaloneSSE: true,
		}, "tunnel_1", ""),
	))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	initID, err := jsonrpc.MakeID("init")
	if err != nil {
		t.Fatal(err)
	}
	initCtx := tunnelTestRequestContext(ctx, "init", initID)
	conn, err := forwarding.Connect(initCtx)
	if err != nil {
		t.Fatal(err)
	}
	initializeTunnelMCP(t, conn, initCtx, initID)
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	callID, err := jsonrpc.MakeID("call")
	if err != nil {
		t.Fatal(err)
	}
	callCtx := tunnelTestRequestContext(ctx, "cmd-call", callID)
	next, err := forwarding.Connect(callCtx)
	if err != nil {
		t.Fatal(err)
	}
	if err := callBridgeProbe(next, callCtx, callID); err != nil {
		t.Fatal(err)
	}
}

func startProbeBridge(t *testing.T) *localmcp.PrivateBridge {
	t.Helper()
	registry := tools.NewRegistry()
	registry.MustRegister("bridge_probe", tools.Schema{
		Name: "bridge_probe", Description: "Probe the private MCP bridge.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}, func(ctx context.Context, _ map[string]any) (tools.Result, error) {
		return tools.JSONResult(map[string]any{"session": tools.MCPSessionID(ctx), "source": tools.CallSource(ctx)}), nil
	})
	bridge, err := localmcp.StartPrivateBridge(&tools.Runtime{Registry: registry, LoopGuard: tools.NewToolLoopGuard()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bridge.Close() })
	return bridge
}

func initializeTunnelMCP(t *testing.T, conn mcpclient.ForwardingConnection, ctx context.Context, id jsonrpc.ID) {
	t.Helper()
	params, err := json.Marshal(map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "cgm-test", "version": "0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(ctx, nil, &jsonrpc.Request{Method: "initialize", ID: id, Params: params}); err != nil {
		t.Fatal(err)
	}
	msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	resp, ok := msg.(*jsonrpc.Response)
	if !ok || resp.Error != nil || resp.ID != id {
		t.Fatalf("initialize = %#v", msg)
	}
	if _, err := conn.Write(ctx, nil, &jsonrpc.Request{Method: "notifications/initialized"}); err != nil {
		t.Fatal(err)
	}
}

func callBridgeProbe(conn mcpclient.ForwardingConnection, ctx context.Context, id jsonrpc.ID) error {
	params, err := json.Marshal(map[string]any{"name": "bridge_probe", "arguments": map[string]any{}})
	if err != nil {
		return err
	}
	if _, err := conn.Write(ctx, nil, &jsonrpc.Request{Method: "tools/call", ID: id, Params: params}); err != nil {
		return err
	}
	msg, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	resp, ok := msg.(*jsonrpc.Response)
	if !ok {
		return errResponse("non-response", msg)
	}
	if resp.Error != nil {
		return errResponse(resp.Error.Error(), resp)
	}
	if resp.ID != id {
		return errResponse("mismatched id", resp)
	}
	return nil
}

type responseError struct {
	message string
	got     any
}

func errResponse(message string, got any) error {
	return &responseError{message: message, got: got}
}

func (e *responseError) Error() string {
	data, _ := json.Marshal(e.got)
	return e.message + ": " + string(data)
}
