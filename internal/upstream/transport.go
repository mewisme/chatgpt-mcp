package upstream

import "context"

type Transport struct {
	kind string
}

func NewStdioTransport() Transport { return Transport{kind: "stdio"} }
func NewHTTPTransport() Transport  { return Transport{kind: "streamable-http"} }

func (t Transport) Connect(ctx context.Context, target string) error { return nil }
