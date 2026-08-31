//go:build linux

package service

import (
	"strings"
	"testing"
)

func TestLinuxUserUnitUsesExplicitConfigAndUserTarget(t *testing.T) {
	spec := Spec{ID: "chatgpt-mcp-user-test", Scope: ScopeUser, ConfigRoot: "/home/mew/.config/chatgpt-mcp", Binary: "/home/mew/.local/bin/cgm", Account: Account{Username: "mew", UID: "1000", GID: "1000", HomeDir: "/home/mew"}}
	unit := LinuxUnit(spec)
	for _, expected := range []string{`ExecStart="/home/mew/.local/bin/cgm" "--config-dir" "/home/mew/.config/chatgpt-mcp" "_service" "run"`, "WantedBy=default.target", "Restart=on-failure"} {
		if !strings.Contains(unit, expected) {
			t.Fatalf("unit missing %q:\n%s", expected, unit)
		}
	}
	if strings.Contains(unit, "User=") {
		t.Fatalf("user unit should inherit user manager identity:\n%s", unit)
	}
}

func TestLinuxSystemUnitRunsAsInvokingUser(t *testing.T) {
	spec := Spec{ID: "chatgpt-mcp-system-test", Scope: ScopeSystem, ConfigRoot: "/home/mew/.config/chatgpt-mcp", Binary: "/usr/local/bin/cgm", Account: Account{Username: "mew", UID: "1000", GID: "1000", HomeDir: "/home/mew"}}
	unit := LinuxUnit(spec)
	for _, expected := range []string{"User=mew", "Group=1000", "WantedBy=multi-user.target"} {
		if !strings.Contains(unit, expected) {
			t.Fatalf("system unit missing %q:\n%s", expected, unit)
		}
	}
	if strings.Contains(unit, "User=root") {
		t.Fatalf("system unit unexpectedly runs MCP as root:\n%s", unit)
	}
}
