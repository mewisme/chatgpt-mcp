package upstream

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseMCPServersJSONInfersMapTransportsAndAliases(t *testing.T) {
	servers, err := ParseMCPServersJSON([]byte(`{"mcpServers":{"local":{"command":"node","args":["./server.js"],"disabled":true},"docs":{"type":"streamable-http","url":"https://example.com/mcp","headers":{"Authorization":"Bearer token"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || servers[0].ID != "docs" || servers[0].Transport != "http" || servers[0].URL != "https://example.com/mcp" || !servers[0].Enabled {
		t.Fatalf("docs=%#v servers=%#v", servers[0], servers)
	}
	if servers[1].ID != "local" || servers[1].Transport != "stdio" || servers[1].Command != "node" || servers[1].Enabled {
		t.Fatalf("local=%#v", servers[1])
	}
}

func TestParseMCPServersJSONSupportsSingleAndArray(t *testing.T) {
	single, err := ParseMCPServersJSON([]byte(`{"id":"local","command":"node"}`))
	if err != nil || len(single) != 1 || single[0].ID != "local" || single[0].Transport != "stdio" {
		t.Fatalf("single=%#v err=%v", single, err)
	}
	array, err := ParseMCPServersJSON([]byte(`[{"id":"a","url":"https://a.example/mcp"},{"id":"b","transport":"stdio","command":"node"}]`))
	if err != nil || len(array) != 2 || array[0].ID != "a" || array[1].ID != "b" {
		t.Fatalf("array=%#v err=%v", array, err)
	}
}

func TestParseMCPServersJSONRejectsAmbiguousAndConflictingTransport(t *testing.T) {
	for _, input := range []string{
		`{"id":"ambiguous","command":"node","url":"https://example.com/mcp"}`,
		`{"id":"conflict","transport":"stdio","type":"http","command":"node"}`,
		`{"id":"missing"}`,
	} {
		if _, err := ParseMCPServersJSON([]byte(input)); err == nil {
			t.Fatalf("accepted invalid input: %s", input)
		}
	}
	explicit, err := ParseMCPServersJSON([]byte(`{"id":"preserve","transport":"stdio","command":"node","url":"https://inactive.example/mcp"}`))
	if err != nil || len(explicit) != 1 || explicit[0].Command != "node" || explicit[0].URL != "https://inactive.example/mcp" {
		t.Fatalf("explicit=%#v err=%v", explicit, err)
	}
}

func TestMCPServersJSONRoundTripPreservesNormalizedServer(t *testing.T) {
	original, err := NormalizeServer(Server{
		ID: "mixed", Name: "Mixed", Transport: "stdio", Enabled: false, Command: "node", URL: "https://inactive.example/mcp", Args: []string{"server.js"},
		Env: map[string]string{"API_TOKEN": "secret", "MODE": "test"}, CWD: "/tmp", Headers: map[string]string{"Authorization": "Bearer secret", "X-Test": "1"},
		BearerTokenEnvVar: "MCP_TOKEN", Auth: AuthConfig{Type: "oauth", Scope: "read write"}, ToolPrefix: "mixed", Expose: "allowlist", Tools: []string{"read"}, DisabledTools: []string{"delete"}, IdleTimeoutSec: 42,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalMCPServersJSON([]Server{original})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseMCPServersJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 || !reflect.DeepEqual(parsed[0], original) {
		t.Fatalf("round trip parsed=%#v original=%#v json=%s", parsed, original, data)
	}
}

func TestMCPServersJSONErrorsDoNotExposeSecretValues(t *testing.T) {
	secret := "TOP_SECRET_VALUE"
	_, err := ParseMCPServersJSON([]byte(`{"mcpServers":{"bad":{"headers":{"Authorization":"` + secret + `"}}}}`))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("secret-safe error=%q", err)
	}
	_, err = ParseMCPServersJSON([]byte(`{"id":"bad","transport":"wat","headers":{"Authorization":"` + secret + `"}}`))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("secret-safe transport error=%q", err)
	}
}

func TestParseMCPServersJSONRejectsDuplicateIDs(t *testing.T) {
	_, err := ParseMCPServersJSON([]byte(`[{"id":"same","command":"node"},{"id":"same","command":"node"}]`))
	if err == nil || !strings.Contains(err.Error(), "duplicate MCP server ID") {
		t.Fatalf("duplicate error=%v", err)
	}
}
