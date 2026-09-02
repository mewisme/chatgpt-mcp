package tunnel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/tunnel-client/pkg/tunnelctx"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func TestSDKBridgePropagatesTunnelSessionID(t *testing.T) {
	registry := tools.NewRegistry()
	seen := make(chan string, 1)
	registry.MustRegister("session_probe", tools.Schema{Name: "session_probe", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(ctx context.Context, _ map[string]any) (tools.Result, error) {
		seen <- tools.MCPSessionID(ctx)
		return tools.TextResult("ok"), nil
	})
	bridge, err := newSDKBridge(&tools.Runtime{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	handler := bridge.toolHandler("session_probe")
	ctx := tunnelctx.ContextWithSessionID(context.Background(), "session-a")
	if _, err := handler(ctx, &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Name: "session_probe", Arguments: json.RawMessage(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	if got := <-seen; got != "session-a" {
		t.Fatalf("session id = %q", got)
	}
}

func TestSDKBridgeConsumesInternalSessionMetaWithoutLoggingIt(t *testing.T) {
	registry := tools.NewRegistry()
	seen := make(chan string, 1)
	registry.MustRegister("session_probe", tools.Schema{Name: "session_probe", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(ctx context.Context, _ map[string]any) (tools.Result, error) {
		seen <- tools.MCPSessionID(ctx)
		return tools.TextResult("ok"), nil
	})
	runtime := &tools.Runtime{Registry: registry}
	observed := make(chan tools.CallObservation, 2)
	runtime.SetCallObserver(func(value tools.CallObservation) { observed <- value })
	bridge, err := newSDKBridge(runtime)
	if err != nil {
		t.Fatal(err)
	}
	handler := bridge.toolHandler("session_probe")
	request := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Name: "session_probe", Arguments: json.RawMessage(`{}`), Meta: sdkmcp.Meta{sessionMetaKey: "session-meta", "client": "keep"}}}
	if _, err := handler(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if got := <-seen; got != "session-meta" {
		t.Fatalf("session id = %q", got)
	}
	if _, exists := request.Params.Meta[sessionMetaKey]; exists {
		t.Fatal("internal session metadata was not consumed")
	}
	for i := 0; i < 2; i++ {
		value := <-observed
		data, err := json.Marshal(value.Raw)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "session-meta") || strings.Contains(string(data), sessionMetaKey) {
			t.Fatalf("raw activity leaked session metadata: %s", data)
		}
	}
}

func TestSDKBridgeCallsSharedToolsRuntime(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister("echo", tools.Schema{
		Name: "echo", Description: "Echo text.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
		Annotations: tools.ToolAnnotations(tools.RiskRead),
	}, func(_ context.Context, args map[string]any) (tools.Result, error) {
		text, _ := args["text"].(string)
		return tools.TextResult("echo:" + text), nil
	})
	runtime := &tools.Runtime{Registry: registry}
	bridge, err := newSDKBridge(runtime)
	if err != nil {
		t.Fatal(err)
	}

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverDone := make(chan error, 1)
	go func() { serverDone <- bridge.Run(ctx, serverTransport) }()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "bridge-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("result = %#v", result)
	}
	text, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok || text.Text != "echo:hello" {
		t.Fatalf("content = %#v", result.Content)
	}

	registry.MustRegister("later", tools.Schema{Name: "later", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, map[string]any) (tools.Result, error) {
		return tools.TextResult("later-ok"), nil
	})
	deadline := time.Now().Add(time.Second)
	for {
		result, err = session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "later", Arguments: map[string]any{}})
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dynamic tool did not reach SDK server: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	text, ok = result.Content[0].(*sdkmcp.TextContent)
	if !ok || text.Text != "later-ok" {
		t.Fatalf("dynamic content = %#v", result.Content)
	}

	cancel()
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("bridge server did not stop")
	}
}

func TestSDKResultPreservesMRTRWireShape(t *testing.T) {
	result, err := sdkResultFromTools(tools.Result{
		ResultType:   "input_required",
		RequestState: "opaque-state",
		InputRequests: map[string]any{
			"confirm": map[string]any{
				"method": "elicitation/create",
				"params": map[string]any{"message": "Continue?", "requestedSchema": map[string]any{"type": "object"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{`"resultType":"input_required"`, `"requestState":"opaque-state"`, `"inputRequests"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %s in %s", expected, text)
		}
	}
	if !strings.Contains(text, `"content":[]`) {
		t.Fatalf("official Go MCP SDK CallToolResult must marshal its required empty content field: %s", text)
	}
}

func TestSDKToolConversionPreservesAnnotationsAndHeaderSchema(t *testing.T) {
	readOnly := true
	schema := tools.Schema{
		Name: "probe", Title: "Probe", Description: "Probe tool.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tenant":{"type":"string","x-mcp-header":"Tenant"}}}`),
		Annotations: map[string]any{"readOnlyHint": readOnly, "openWorldHint": false},
	}
	tool, err := sdkToolFromSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	if tool.Name != "probe" || tool.Title != "Probe" || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Fatalf("tool = %#v", tool)
	}
	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"x-mcp-header":"Tenant"`) {
		t.Fatalf("input schema lost x-mcp-header: %s", data)
	}
}
