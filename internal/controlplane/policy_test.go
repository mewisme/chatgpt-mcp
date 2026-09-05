package controlplane

import "testing"

func TestReadOnlyCommandPolicy(t *testing.T) {
	for _, args := range [][]string{
		{"status"}, {"config", "list"}, {"config", "preset", "show", "default"}, {"auth", "status"},
		{"workspace", "access", "list", "ws_test"}, {"mcp", "server", "show", "server"}, {"tunnel", "status"}, {"alias", "status"}, {"update", "check"},
		{"request", "list"}, {"request", "view", "req_test"}, {"req", "ls"}, {"req", "show", "req_test"}, {"req", "info", "req_test"},
		{"st"}, {"cfg", "ls"}, {"ws", "access", "ls", "ws_test"}, {"mcp", "server", "st", "server"}, {"tunnel", "st"}, {"completion", "bash"},
		{"--config-dir", "/tmp/config", "config", "get", "server.expose"}, {"--verbose", "status"}, {"--help"},
	} {
		if !IsReadOnlyArgs(args) {
			t.Fatalf("read-only command denied: %#v -> %q", args, PathFromArgs(args))
		}
	}
	for _, args := range [][]string{
		{"config", "set", "permissions.allow_dirs", "/tmp"}, {"config", "convert", "yaml"}, {"config", "export", "backup.cgm"}, {"config", "import", "backup.cgm"}, {"config", "preset", "apply", "lan"},
		{"cfg", "set", "permissions.allow_dirs", "/tmp"}, {"ws", "register", "."},
		{"auth", "mcp", "create"}, {"workspace", "register", "."}, {"workspace", "access", "add", "ws_test", "/tmp"},
		{"request", "approve", "req_test"}, {"request", "deny", "req_test"}, {"req", "accept", "req_test"}, {"req", "allow", "req_test"}, {"req", "reject", "req_test"},
		{"mcp", "server", "add", "server"}, {"tunnel", "enable"}, {"alias", "install"}, {"alias", "remove"}, {"update"}, {"serve"}, {},
	} {
		if IsReadOnlyArgs(args) {
			t.Fatalf("mutating command allowed: %#v -> %q", args, PathFromArgs(args))
		}
	}
}

func TestApprovalEligibleCommandPolicy(t *testing.T) {
	for _, args := range [][]string{
		{"update"}, {"install"}, {"config", "set", "server.port", "41001"}, {"workspace", "access", "add", "ws_test", "/tmp"},
	} {
		if !ApprovalEligibleArgs(args) {
			t.Fatalf("approval-eligible command denied: %#v -> %q", args, PathFromArgs(args))
		}
	}
	for _, args := range [][]string{
		{"status"}, {"update", "check"}, {"request", "approve", "req_test"}, {"request", "deny", "req_test"}, {"req", "accept", "req_test"}, {"req", "reject", "req_test"}, {"request", "list"}, {"request", "view", "req_test"}, {"_service", "run"}, {},
	} {
		if ApprovalEligibleArgs(args) {
			t.Fatalf("hard-denied/read-only command became approval eligible: %#v -> %q", args, PathFromArgs(args))
		}
	}
}
