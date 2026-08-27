package upstream

import "context"

type Client interface {
	Connect(context.Context, Server) error
	Close(context.Context, string) error
	Tools(context.Context, string) ([]Tool, error)
	Call(context.Context, string, string, map[string]any) (any, error)
}

type Tool struct {
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}
