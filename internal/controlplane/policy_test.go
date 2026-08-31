package controlplane

import "testing"

func TestReadOnlyCommandPolicy(t *testing.T) {
	for _, args := range [][]string{
		{"status"}, {"config", "list"}, {"config", "preset", "show", "default"}, {"auth", "status"},
		{"workspace", "access", "list", "ws_test"}, {"mcp", "server", "show", "server"}, {"tunnel", "status"},
		{"st"}, {"cfg", "ls"}, {"ws", "access", "ls", "ws_test"}, {"mcp", "server", "st", "server"}, {"tunnel", "st"},
		{"--config-dir", "/tmp/config", "config", "get", "server.expose"}, {"--verbose", "status"}, {"--help"},
	} {
		if !IsReadOnlyArgs(args) {
			t.Fatalf("read-only command denied: %#v -> %q", args, PathFromArgs(args))
		}
	}
	for _, args := range [][]string{
		{"config", "set", "permissions.allow_dirs", "/tmp"}, {"config", "convert", "yaml"}, {"config", "preset", "apply", "lan"},
		{"cfg", "set", "permissions.allow_dirs", "/tmp"}, {"ws", "register", "."},
		{"auth", "mcp", "create"}, {"workspace", "register", "."}, {"workspace", "access", "add", "ws_test", "/tmp"},
		{"mcp", "server", "add", "server"}, {"tunnel", "enable"}, {"serve"}, {},
	} {
		if IsReadOnlyArgs(args) {
			t.Fatalf("mutating command allowed: %#v -> %q", args, PathFromArgs(args))
		}
	}
}
