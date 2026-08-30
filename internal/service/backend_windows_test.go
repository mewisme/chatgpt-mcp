//go:build windows

package service

import (
	"strings"
	"testing"
)

func TestWindowsTaskIsPerUserLeastPrivilege(t *testing.T) {
	spec := Spec{ID: "chatgpt-mcp-user-test", Scope: ScopeUser, ConfigRoot: `C:\Users\Mew\.config\chatgpt-mcp`, Binary: `C:\Users\Mew\AppData\Local\chatgpt-mcp\bin\chatgpt-mcp.exe`, Account: Account{Username: `PC\Mew`, HomeDir: `C:\Users\Mew`}}
	xml := WindowsTaskXML(spec)
	for _, expected := range []string{"<LogonType>InteractiveToken</LogonType>", "<RunLevel>LeastPrivilege</RunLevel>", "<RestartOnFailure>", "--config-dir", `C:\Users\Mew\.config\chatgpt-mcp`} {
		if !strings.Contains(xml, expected) {
			t.Fatalf("task XML missing %q:\n%s", expected, xml)
		}
	}
	for _, forbidden := range []string{"SYSTEM", "HighestAvailable", "Password"} {
		if strings.Contains(xml, forbidden) {
			t.Fatalf("task XML contains forbidden %q:\n%s", forbidden, xml)
		}
	}
}
