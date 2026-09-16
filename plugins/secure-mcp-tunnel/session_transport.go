package securemcptunnel

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/tunnel-client/pkg/tunnelctx"

	localmcp "go.mewis.me/chatgpt-mcp/internal/mcp"
)

const sessionMetaKey = localmcp.SessionMetaKey

type sessionTransport struct {
	base       sdkmcp.Transport
	onActivity func()
	tunnelID   string
	tunnelName string
}

type sessionConnection struct {
	base       sdkmcp.Connection
	onActivity func()
	tunnelID   string
	tunnelName string
}

func withSessionTransport(base sdkmcp.Transport, tunnelID, tunnelName string) sdkmcp.Transport {
	return withSessionTransportActivity(base, tunnelID, tunnelName, nil)
}

func withSessionTransportActivity(base sdkmcp.Transport, tunnelID, tunnelName string, onActivity func()) sdkmcp.Transport {
	if base == nil {
		return nil
	}
	return &sessionTransport{base: base, onActivity: onActivity, tunnelID: tunnelID, tunnelName: tunnelName}
}

func (t *sessionTransport) Connect(ctx context.Context) (sdkmcp.Connection, error) {
	conn, err := t.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &sessionConnection{base: conn, onActivity: t.onActivity, tunnelID: t.tunnelID, tunnelName: t.tunnelName}, nil
}

func (c *sessionConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	return c.base.Read(ctx)
}

func (c *sessionConnection) Write(ctx context.Context, msg jsonrpc.Message) error {
	request, isRequest := msg.(*jsonrpc.Request)
	if isRequest && request != nil && request.Method == "tools/call" {
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
		if sessionID, ok := tunnelctx.SessionIDFromContext(ctx); ok {
			meta[sessionMetaKey] = sessionID
		}
		if c.tunnelID != "" {
			meta[localmcp.TunnelIDMetaKey] = c.tunnelID
		}
		if c.tunnelName != "" {
			meta[localmcp.TunnelNameMetaKey] = c.tunnelName
		}
		encoded, err := json.Marshal(params)
		if err != nil {
			return err
		}
		request.Params = encoded
	}
	err := c.base.Write(ctx, msg)
	if err == nil && isRequest && request != nil && c.onActivity != nil {
		c.onActivity()
	}
	return err
}

func (c *sessionConnection) Close() error      { return c.base.Close() }
func (c *sessionConnection) SessionID() string { return c.base.SessionID() }
