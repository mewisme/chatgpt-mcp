package mcp

import (
	"context"
	"io"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

type StdioRuntime struct {
	Server *SDKServer
	In     io.ReadCloser
	Out    io.WriteCloser
}

func NewStdioRuntime(toolRuntime *tools.Runtime, in io.ReadCloser, out io.WriteCloser) (*StdioRuntime, error) {
	server, err := NewSDKServerWithSession(toolRuntime, "stdio", auth.GenerateToken("stdio"))
	if err != nil {
		return nil, err
	}
	return &StdioRuntime{Server: server, In: in, Out: out}, nil
}

func (r *StdioRuntime) Run(ctx context.Context) error {
	return r.Server.Server.Run(ctx, &sdkmcp.IOTransport{Reader: r.In, Writer: r.Out})
}
