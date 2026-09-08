package page

import (
	"reflect"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

func TestMCPServerFormCreatesNormalizedHTTPServer(t *testing.T) {
	_, data := newMCPServerForm(upstream.Server{}, true)
	data.ID = " docs "
	data.Name = "Docs"
	data.Transport = "http"
	data.URL = " https://example.test/mcp "
	data.Headers = "X-Mode=read"
	data.SensitiveHeaders = `{"Authorization":"Bearer secret"}`
	data.AuthType = "oauth"
	data.AuthScope = "read write"
	data.Expose = "allowlist"
	data.Tools = "read\nsearch"
	data.DisabledTools = "delete"
	data.IdleTimeout = "45"
	server, err := serverFromMCPForm(data, upstream.Server{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if server.ID != "docs" || server.Name != "Docs" || server.URL != "https://example.test/mcp" || server.ToolPrefix != "docs" || server.IdleTimeoutSec != 45 {
		t.Fatalf("normalized server=%#v", server)
	}
	if want := map[string]string{"X-Mode": "read", "Authorization": "Bearer secret"}; !reflect.DeepEqual(server.Headers, want) {
		t.Fatalf("headers=%#v want %#v", server.Headers, want)
	}
	if want := []string{"read", "search"}; !reflect.DeepEqual(server.Tools, want) {
		t.Fatalf("tools=%#v want %#v", server.Tools, want)
	}
}

func TestMCPServerFormPreservesExistingSecretsWithoutRenderingThem(t *testing.T) {
	existing := upstream.Server{
		ID: "demo", Name: "Demo", Enabled: true, Transport: "http", URL: "https://example.test/mcp", Expose: "all",
		Headers: map[string]string{"Authorization": "Bearer top-secret", "X-Mode": "read"},
		Env:     map[string]string{"API_TOKEN": "env-secret", "MODE": "prod"},
	}
	form, data := newMCPServerForm(existing, false)
	if view := form.View(); strings.Contains(view, "top-secret") || strings.Contains(view, "env-secret") {
		t.Fatalf("existing secret rendered in form: %q", view)
	}
	server, err := serverFromMCPForm(data, existing, false)
	if err != nil {
		t.Fatal(err)
	}
	if server.Headers["Authorization"] != "Bearer top-secret" || server.Env["API_TOKEN"] != "env-secret" {
		t.Fatalf("existing secrets were not preserved: %#v %#v", server.Headers, server.Env)
	}
}

func TestMCPServerFormSeparatesSensitiveAssignments(t *testing.T) {
	existing := upstream.Server{ID: "demo", Transport: "http", URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "old"}, Expose: "all"}
	_, data := newMCPServerForm(existing, false)
	data.Headers = "Authorization=visible"
	if _, err := serverFromMCPForm(data, existing, false); err == nil || !strings.Contains(err.Error(), "masked JSON") {
		t.Fatalf("sensitive plaintext error=%v", err)
	}
	data.Headers = "X-Mode=read"
	data.SensitiveHeaders = `{"X-Unsafe":"value"}`
	if _, err := serverFromMCPForm(data, existing, false); err == nil || !strings.Contains(err.Error(), "regular assignments") {
		t.Fatalf("non-sensitive masked error=%v", err)
	}
	data.SensitiveHeaders = `{}`
	server, err := serverFromMCPForm(data, existing, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := server.Headers["Authorization"]; exists {
		t.Fatalf("empty sensitive JSON did not remove existing secret: %#v", server.Headers)
	}
}

func TestMCPServerFormPreservesInactiveTransportDraftValues(t *testing.T) {
	_, data := newMCPServerForm(upstream.Server{}, true)
	data.ID = "mixed"
	data.Name = "Mixed"
	data.Transport = "stdio"
	data.Command = "node"
	data.Args = "server.js"
	data.CWD = "/tmp/project"
	data.Env = "MODE=dev"
	data.URL = "https://inactive.example/mcp"
	data.Headers = "X-Inactive=kept"
	data.BearerTokenEnvVar = "MCP_TOKEN"
	data.AuthType = "none"
	server, err := serverFromMCPForm(data, upstream.Server{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if server.Transport != "stdio" || server.Command != "node" || server.URL != "https://inactive.example/mcp" || server.Headers["X-Inactive"] != "kept" || server.BearerTokenEnvVar != "MCP_TOKEN" {
		t.Fatalf("inactive HTTP draft was lost: %#v", server)
	}
	data.Transport = "http"
	data.AuthType = "auto"
	server, err = serverFromMCPForm(data, upstream.Server{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if server.Transport != "http" || server.URL != "https://inactive.example/mcp" || server.Command != "node" || len(server.Args) != 1 || server.Args[0] != "server.js" || server.Env["MODE"] != "dev" {
		t.Fatalf("inactive stdio draft was lost: %#v", server)
	}
}
