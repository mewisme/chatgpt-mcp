package controlplane

import "testing"

func TestReadOnlyCommandPolicy(t *testing.T) {
	for _, args := range [][]string{
		{"status"}, {"config", "list"}, {"config", "preset", "show", "default"}, {"auth", "status"},
		{"workspace", "access", "list", "ws_test"}, {"mcp", "server", "show", "server"}, {"tunnel", "status"},
		{"--config-dir", "/tmp/config", "config", "get", "server.expose"}, {"--verbose", "status"}, {"--help"},
	} {
		if !IsReadOnlyArgs(args) {
			t.Fatalf("read-only command denied: %#v -> %q", args, PathFromArgs(args))
		}
	}
	for _, args := range [][]string{
		{"config", "set", "permissions.allow_dirs", "/tmp"}, {"config", "convert", "yaml"}, {"config", "preset", "apply", "lan"},
		{"auth", "mcp", "create"}, {"workspace", "register", "."}, {"workspace", "access", "add", "ws_test", "/tmp"},
		{"mcp", "server", "add", "server"}, {"tunnel", "enable"}, {"serve"}, {},
	} {
		if IsReadOnlyArgs(args) {
			t.Fatalf("mutating command allowed: %#v -> %q", args, PathFromArgs(args))
		}
	}
}
