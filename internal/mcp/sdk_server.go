package mcp

import (
	"context"
	"encoding/json"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

type SDKServer struct {
	Server    *sdkmcp.Server
	Tools     *tools.Runtime
	Source    string
	SessionID string
}

func NewSDKServerWithTools(toolRuntime *tools.Runtime, source string) (*SDKServer, error) {
	return NewSDKServerWithSession(toolRuntime, source, "")
}

func NewSDKServerWithSession(toolRuntime *tools.Runtime, source, sessionID string) (*SDKServer, error) {
	if toolRuntime == nil {
		toolRuntime = tools.NewRuntime()
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "chatgpt-mcp", Version: version.Version}, &sdkmcp.ServerOptions{Capabilities: &sdkmcp.ServerCapabilities{Tools: &sdkmcp.ToolCapabilities{ListChanged: true}}})
	adapter := &SDKServer{Server: server, Tools: toolRuntime, Source: source, SessionID: sessionID}
	for _, schema := range filterHeaderSafeTools(toolRuntime.List()) {
		if err := adapter.addTool(schema); err != nil {
			return nil, err
		}
	}
	return adapter, nil
}

func (s *SDKServer) addTool(schema tools.Schema) error {
	tool := &sdkmcp.Tool{Name: schema.Name, Title: schema.Title, Description: schema.Description, InputSchema: schema.InputSchema, OutputSchema: schema.OutputSchema}
	if len(schema.Annotations) > 0 {
		data, err := json.Marshal(schema.Annotations)
		if err != nil {
			return err
		}
		annotations := &sdkmcp.ToolAnnotations{}
		if err := json.Unmarshal(data, annotations); err != nil {
			return err
		}
		tool.Annotations = annotations
	}
	s.Server.AddTool(tool, func(ctx context.Context, request *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		args := map[string]any{}
		if len(request.Params.Arguments) > 0 {
			if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
				return nil, err
			}
		}
		sessionID := s.SessionID
		if sessionID == "" && request.Session != nil {
			sessionID = request.Session.ID()
		}
		if sessionID != "" {
			ctx = tools.WithMCPSessionID(ctx, sessionID)
		}
		ctx = tools.WithCallSource(ctx, s.Source)
		ctx = tools.WithInputRound(ctx, request.Params.RequestState, inputResponses(request.Params.InputResponses))
		result, err := s.Tools.Call(ctx, schema.Name, args)
		if errors.Is(err, tools.ErrToolNotFound) {
			return nil, err
		}
		if err != nil {
			return nil, err
		}
		return sdkCallToolResult(result)
	})
	return nil
}

func sdkCallToolResult(result tools.Result) (*sdkmcp.CallToolResult, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var converted sdkmcp.CallToolResult
	if err := json.Unmarshal(data, &converted); err != nil {
		return nil, err
	}
	return &converted, nil
}

func inputResponses(values sdkmcp.InputResponseMap) map[string]any {
	if len(values) == 0 {
		return nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	result := map[string]any{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}
