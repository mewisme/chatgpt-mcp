//go:build windows

package service

import (
	"fmt"
	"os"
	"strings"
)

type windowsManager struct{}

func NewManager() Manager              { return windowsManager{} }
func (windowsManager) Backend() string { return "task-scheduler" }

func (windowsManager) DefinitionMatches(spec Spec) (bool, error) {
	output, ok := commandSucceeded("schtasks.exe", "/Query", "/TN", windowsTaskName(spec), "/XML")
	if !ok {
		return false, nil
	}
	return strings.Contains(output, "<Command>"+xmlText(spec.Binary)+"</Command>") && strings.Contains(output, "<Arguments>"+xmlText(windowsCommandLine(Args(spec)))+"</Arguments>"), nil
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
	if _, err := file.WriteString(WindowsTaskXML(spec)); err != nil {
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

func WindowsTaskXML(spec Spec) string {
	arguments := windowsCommandLine(Args(spec))
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
      <Interval>PT3S</Interval>
      <Count>5</Count>
    </RestartOnFailure>
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
`, xmlText(spec.Account.Username), xmlText(spec.Account.Username), xmlText(spec.Binary), xmlText(arguments), xmlText(spec.Account.HomeDir))
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
