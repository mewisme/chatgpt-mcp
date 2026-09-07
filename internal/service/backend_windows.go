//go:build windows

package service

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

type windowsManager struct{}

func NewManager() Manager              { return windowsManager{} }
func (windowsManager) Backend() string { return "task-scheduler" }

func (windowsManager) DefinitionMatches(spec Spec) (bool, error) {
	command, err := windowsTaskCommand()
	if err != nil {
		return false, err
	}
	output, ok := commandSucceeded("schtasks.exe", "/Query", "/TN", windowsTaskName(spec), "/XML")
	if !ok {
		return false, nil
	}
	return strings.Contains(output, "<Command>"+xmlText(command)+"</Command>") && strings.Contains(output, "<Arguments>"+xmlText(windowsTaskArguments(spec))+"</Arguments>") && strings.Contains(output, "<Hidden>true</Hidden>"), nil
}

func (windowsManager) Install(spec Spec) error {
	if err := os.MkdirAll(spec.ConfigRoot, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(spec.ConfigRoot, ".service-task-*.xml")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	xml, err := WindowsTaskXML(spec)
	if err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.WriteString(xml); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, err = runCommand("schtasks.exe", "/Create", "/TN", windowsTaskName(spec), "/XML", path, "/F")
	return err
}

func (windowsManager) Start(spec Spec) error {
	_, err := runCommand("schtasks.exe", "/Run", "/TN", windowsTaskName(spec))
	return err
}

func (windowsManager) Stop(spec Spec) error {
	_, err := runCommand("schtasks.exe", "/End", "/TN", windowsTaskName(spec))
	return err
}

func (windowsManager) Uninstall(spec Spec) error {
	_, err := runCommand("schtasks.exe", "/Delete", "/TN", windowsTaskName(spec), "/F")
	return err
}

func (windowsManager) Status(spec Spec) (Status, error) {
	_, installed := commandSucceeded("schtasks.exe", "/Query", "/TN", windowsTaskName(spec))
	return Status{Installed: installed, Backend: "task-scheduler"}, nil
}

func WindowsTaskXML(spec Spec) (string, error) {
	command, err := windowsTaskCommand()
	if err != nil {
		return "", err
	}
	arguments := windowsTaskArguments(spec)
	return fmt.Sprintf(`<?xml version="1.0"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>ChatGPT MCP managed runtime</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>%s</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%s</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <StartWhenAvailable>true</StartWhenAvailable>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>5</Count>
    </RestartOnFailure>
    <Hidden>true</Hidden>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>%s</Arguments>
      <WorkingDirectory>%s</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`, xmlText(spec.Account.Username), xmlText(spec.Account.Username), xmlText(command), xmlText(arguments), xmlText(spec.Account.HomeDir)), nil
}

func windowsTaskCommand() (string, error) {
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve Windows system directory: %w", err)
	}
	return filepath.Join(systemDir, "WindowsPowerShell", "v1.0", "powershell.exe"), nil
}

func windowsTaskArguments(spec Spec) string {
	script := windowsHiddenProcessScript(spec)
	return windowsCommandLine([]string{"-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", windowsPowerShellEncodedCommand(script)})
}

func windowsHiddenProcessScript(spec Spec) string {
	arguments := windowsCommandLine(Args(spec))
	return strings.Join([]string{
		"$psi = New-Object System.Diagnostics.ProcessStartInfo",
		"$psi.FileName = " + windowsPowerShellString(spec.Binary),
		"$psi.Arguments = " + windowsPowerShellString(arguments),
		"$psi.WorkingDirectory = " + windowsPowerShellString(spec.Account.HomeDir),
		"$psi.UseShellExecute = $false",
		"$psi.CreateNoWindow = $true",
		"$psi.WindowStyle = [System.Diagnostics.ProcessWindowStyle]::Hidden",
		"$process = [System.Diagnostics.Process]::Start($psi)",
		"$process.WaitForExit()",
		"exit $process.ExitCode",
	}, "; ")
}

func windowsPowerShellString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func windowsPowerShellEncodedCommand(script string) string {
	encoded := utf16.Encode([]rune(script))
	bytes := make([]byte, len(encoded)*2)
	for i, value := range encoded {
		bytes[i*2] = byte(value)
		bytes[i*2+1] = byte(value >> 8)
	}
	return base64.StdEncoding.EncodeToString(bytes)
}

func windowsTaskName(spec Spec) string { return spec.ID }

func windowsCommandLine(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = windowsQuoteArg(arg)
	}
	return strings.Join(quoted, " ")
}

func windowsQuoteArg(value string) string {
	if value == "" {
		return `""`
	}
	if !strings.ContainsAny(value, " \t\"") {
		return value
	}
	var builder strings.Builder
	builder.WriteByte('"')
	backslashes := 0
	for _, char := range value {
		if char == '\\' {
			backslashes++
			continue
		}
		if char == '"' {
			builder.WriteString(strings.Repeat(`\`, backslashes*2+1))
			builder.WriteRune(char)
			backslashes = 0
			continue
		}
		builder.WriteString(strings.Repeat(`\`, backslashes))
		backslashes = 0
		builder.WriteRune(char)
	}
	builder.WriteString(strings.Repeat(`\`, backslashes*2))
	builder.WriteByte('"')
	return builder.String()
}
