//go:build windows

package service

import (
	"strings"
	"testing"
)

func TestWindowsTaskIsPerUserLeastPrivilege(t *testing.T) {
	spec := Spec{ID: "chatgpt-mcp-user-test", Scope: ScopeUser, ConfigRoot: `C:\Users\Mew\.config\chatgpt-mcp`, Binary: `C:\Users\Mew\AppData\Local\chatgpt-mcp\bin\chatgpt-mcp.exe`, Account: Account{Username: `PC\Mew`, HomeDir: `C:\Users\Mew`}}
	xml, err := WindowsTaskXML(spec)
	if err != nil {
		t.Fatal(err)
	}
	command, err := windowsTaskCommand()
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"<LogonType>InteractiveToken</LogonType>", "<RunLevel>LeastPrivilege</RunLevel>", "<RestartOnFailure>", "<Command>" + xmlText(command) + "</Command>", "-WindowStyle Hidden", "--config-dir", `C:\Users\Mew\.config\chatgpt-mcp`, "<Interval>PT1M</Interval>"} {
		if !strings.Contains(xml, expected) {
			t.Fatalf("task XML missing %q:\n%s", expected, xml)
		}
	}
	for _, forbidden := range []string{"SYSTEM", "HighestAvailable", "Password", "encoding=", "<Interval>PT3S</Interval>", "<Command>powershell.exe</Command>"} {
		if strings.Contains(xml, forbidden) {
			t.Fatalf("task XML contains forbidden %q:\n%s", forbidden, xml)
		}
	}
}
