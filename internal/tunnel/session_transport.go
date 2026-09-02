package tunnel

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/tunnel-client/pkg/tunnelctx"
)

const sessionMetaKey = "go.mewis.me/chatgpt-mcp/mcp-session-id"

type sessionTransport struct{ base sdkmcp.Transport }

type sessionConnection struct{ base sdkmcp.Connection }

func withSessionTransport(base sdkmcp.Transport) sdkmcp.Transport {
	if base == nil {
		return nil
	}
	return &sessionTransport{base: base}
}

func (t *sessionTransport) Connect(ctx context.Context) (sdkmcp.Connection, error) {
	conn, err := t.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &sessionConnection{base: conn}, nil
}

func (c *sessionConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	return c.base.Read(ctx)
}

func (c *sessionConnection) Write(ctx context.Context, msg jsonrpc.Message) error {
	if sessionID, ok := tunnelctx.SessionIDFromContext(ctx); ok {
		if request, ok := msg.(*jsonrpc.Request); ok && request != nil && request.Method == "tools/call" {
			params := map[string]any{}
			if len(request.Params) > 0 && string(request.Params) != "null" {
				if err := json.Unmarshal(request.Params, &params); err != nil {
					return err
				}
			}
			meta, _ := params["_meta"].(map[string]any)
			if meta == nil {
				meta = map[string]any{}
				params["_meta"] = meta
			}
			meta[sessionMetaKey] = sessionID
			encoded, err := json.Marshal(params)
			if err != nil {
				return err
			}
			request.Params = encoded
		}
	}
	return c.base.Write(ctx, msg)
}

func (c *sessionConnection) Close() error      { return c.base.Close() }
func (c *sessionConnection) SessionID() string { return c.base.SessionID() }
