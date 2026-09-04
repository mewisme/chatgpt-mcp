package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRootLogFormatJSON(t *testing.T) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(testCommandArgs(t, "--log-format=json", "version"))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("output %q is not JSON: %v", output.String(), err)
	}
	if event["component"] != "VERSION" || event["level"] != "info" || event["message"] == "" {
		t.Fatalf("event = %#v", event)
	}
}

func TestRootDebugUsesDiagnosticRenderer(t *testing.T) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(testCommandArgs(t, "--debug", "version"))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "INF") || !strings.Contains(text, "VERSION") {
		t.Fatalf("debug output = %q", text)
	}
}

func TestRootRejectsInvalidLogFormat(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs(testCommandArgs(t, "--log-format=yaml", "version"))
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unsupported log format") {
		t.Fatalf("error = %v", err)
	}
}

func TestCommandLoggerJSONFailure(t *testing.T) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(testCommandArgs(t, "--log-format=json", "version"))
	if err := cmd.ParseFlags([]string{"--log-format=json"}); err != nil {
		t.Fatal(err)
	}
	commandLogger(cmd).Failure("CLI", "cli.command.failed", "Command failed", errors.New("boom"))
	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("output %q is not JSON: %v", output.String(), err)
	}
	if event["event"] != "cli.command.failed" || event["error"] != "boom" {
		t.Fatalf("event = %#v", event)
	}
}

func TestStartCommandSpinnerIsSilentWithoutTerminal(t *testing.T) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	log := commandLogger(cmd)
	startCommandSpinner(cmd, log, "TEST", "test.waiting", "Waiting")
	log.Close()
	if output.Len() != 0 {
		t.Fatalf("spinner wrote to non-terminal output: %q", output.String())
	}
}
