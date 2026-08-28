package cli

import (
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

func TestParseAssignments(t *testing.T) {
	value, err := parseAssignments([]string{"A=1", "B=two=parts"}, "env")
	if err != nil {
		t.Fatal(err)
	}
	if value["A"] != "1" || value["B"] != "two=parts" {
		t.Fatalf("value = %#v", value)
	}
	if _, err := parseAssignments([]string{"broken"}, "env"); err == nil {
		t.Fatal("invalid assignment was accepted")
	}
}

func TestRedactUpstreamServer(t *testing.T) {
	server := upstream.Server{
		Headers: map[string]string{"Authorization": "Bearer secret", "X-Test": "ok"},
		Env:     map[string]string{"API_TOKEN": "secret", "MODE": "test"},
	}
	value := redactUpstreamServer(server)
	if value.Headers["Authorization"] != "<redacted>" || value.Headers["X-Test"] != "ok" {
		t.Fatalf("headers = %#v", value.Headers)
	}
	if value.Env["API_TOKEN"] != "<redacted>" || value.Env["MODE"] != "test" {
		t.Fatalf("env = %#v", value.Env)
	}
	if server.Headers["Authorization"] != "Bearer secret" || server.Env["API_TOKEN"] != "secret" {
		t.Fatal("redaction mutated source config")
	}
}
