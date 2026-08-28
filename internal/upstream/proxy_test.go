package upstream

import (
	"context"
	"testing"
)

func TestProxiedNamesRespectExposePolicy(t *testing.T) {
	manager := NewManagerWithClient(nil, &fakeClient{})
	server, err := NormalizeServer(Server{
		ID: "demo", Name: "Demo", Enabled: true, Transport: "http", URL: "http://example.test",
		Expose: "allowlist", Tools: []string{"read"}, DisabledTools: []string{"delete"},
	})
	if err != nil {
		t.Fatal(err)
	}
	values := []Tool{{Name: "delete"}, {Name: "read"}, {Name: "write"}}
	names := manager.ProxiedToolNames(server, values)
	if len(names) != 1 || names[0] != "demo__read" {
		t.Fatalf("names = %#v", names)
	}
}

type fakeClient struct{}

func (*fakeClient) Connect(context.Context, Server) error         { return nil }
func (*fakeClient) Close(context.Context, string) error           { return nil }
func (*fakeClient) Tools(context.Context, string) ([]Tool, error) { return nil, nil }
func (*fakeClient) Call(context.Context, string, string, map[string]any) (CallResult, error) {
	return CallResult{}, nil
}
func (*fakeClient) PID(string) int { return 0 }
