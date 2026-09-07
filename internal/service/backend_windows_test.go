//go:build windows

package service

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf16"
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
	for _, expected := range []string{"<LogonType>InteractiveToken</LogonType>", "<RunLevel>LeastPrivilege</RunLevel>", "<RestartOnFailure>", "<Command>" + xmlText(command) + "</Command>", "-WindowStyle Hidden", "-EncodedCommand", "<Hidden>true</Hidden>", "<Interval>PT1M</Interval>"} {
		if !strings.Contains(xml, expected) {
			t.Fatalf("task XML missing %q:\n%s", expected, xml)
		}
	}
	for _, forbidden := range []string{"SYSTEM", "HighestAvailable", "Password", "encoding=", "<Interval>PT3S</Interval>", "<Command>powershell.exe</Command>", " -Command "} {
		if strings.Contains(xml, forbidden) {
			t.Fatalf("task XML contains forbidden %q:\n%s", forbidden, xml)
		}
	}
}

func TestWindowsTaskLaunchesManagedRuntimeWithoutConsoleWindow(t *testing.T) {
	spec := Spec{ID: "chatgpt-mcp-user-test", Scope: ScopeUser, ConfigRoot: `C:\Users\Mew\.config\chatgpt-mcp`, Binary: `C:\Program Files\chatgpt-mcp\chatgpt-mcp.exe`, EnvironmentHash: "env_test", Account: Account{Username: `PC\Mew`, HomeDir: `C:\Users\Mew`}}
	arguments := windowsTaskArguments(spec)
	parts := strings.Fields(arguments)
	if len(parts) == 0 {
		t.Fatal("empty task arguments")
	}
	decoded, err := base64.StdEncoding.DecodeString(parts[len(parts)-1])
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded)%2 != 0 {
		t.Fatalf("encoded PowerShell command has odd byte length: %d", len(decoded))
	}
	units := make([]uint16, len(decoded)/2)
	for i := range units {
		units[i] = uint16(decoded[i*2]) | uint16(decoded[i*2+1])<<8
	}
	script := string(utf16.Decode(units))
	for _, expected := range []string{"$psi.UseShellExecute = $false", "$psi.CreateNoWindow = $true", "$psi.WindowStyle = [System.Diagnostics.ProcessWindowStyle]::Hidden", "$process.WaitForExit()", "exit $process.ExitCode", spec.Binary, "--config-dir", spec.ConfigRoot, "--service-environment-hash", spec.EnvironmentHash} {
		if !strings.Contains(script, expected) {
			t.Fatalf("hidden launcher script missing %q:\n%s", expected, script)
		}
	}
	if strings.Contains(script, "Start-Process") {
		t.Fatalf("hidden launcher should use ProcessStartInfo directly:\n%s", script)
	}
}
