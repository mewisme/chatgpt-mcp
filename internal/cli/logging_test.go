package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

func TestExecuteCommandVerboseEmitsLifecycle(t *testing.T) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(testCommandArgs(t, "--verbose", "version"))
	if err := executeCommand(cmd); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"Executing command", "Command completed", "command:"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("verbose lifecycle missing %q: %s", expected, text)
		}
	}
	if strings.Contains(text, "error_chain") || strings.Contains(text, "changed_flags") {
		t.Fatalf("verbose output leaked debug-only fields: %s", text)
	}
}

func TestExecuteCommandDebugFailureEmitsDiagnostics(t *testing.T) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.AddCommand(&cobra.Command{Use: "explode", RunE: func(*cobra.Command, []string) error { return errors.Join(errors.New("outer"), errors.New("inner")) }})
	cmd.SetArgs(testCommandArgs(t, "--debug", "explode"))
	err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected failure")
	}
	text := output.String()
	for _, expected := range []string{"cli.command.starting", "cli.command.context", "cli.command.failed", "error_type=", "error_chain=", "changed_flags=", "duration_ms="} {
		if !strings.Contains(text, expected) {
			t.Fatalf("debug failure missing %q: %s", expected, text)
		}
	}
}

func TestExecuteCommandDebugDoesNotLogFlagValues(t *testing.T) {
	var output bytes.Buffer
	var token string
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	child := &cobra.Command{Use: "secret", RunE: func(*cobra.Command, []string) error { return errors.New("expected failure") }}
	child.Flags().StringVar(&token, "token", "", "test secret")
	cmd.AddCommand(child)
	cmd.SetArgs(testCommandArgs(t, "--debug", "secret", "--token", "supersecret-value"))
	if err := executeCommand(cmd); err == nil {
		t.Fatal("expected failure")
	}
	text := output.String()
	if !strings.Contains(text, "--token") {
		t.Fatalf("debug output did not identify changed flag: %s", text)
	}
	if strings.Contains(text, "supersecret-value") {
		t.Fatalf("debug output leaked flag value: %s", text)
	}
}

func TestMachineJSONOutputKeepsDiagnosticsOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var asJSON bool
	cmd := newRootCommand()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	child := &cobra.Command{Use: "machine", RunE: func(cmd *cobra.Command, _ []string) error {
		logCommandStep(cmd, "TEST", "test.machine.loading", "Loading machine output")
		return printJSON(cmd, map[string]any{"ok": true})
	}}
	addJSONOutputFlag(child, &asJSON)
	cmd.AddCommand(child)
	cmd.SetArgs(testCommandArgs(t, "--verbose", "machine", "--json"))
	if err := executeCommand(cmd); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &value); err != nil || value["ok"] != true {
		t.Fatalf("stdout is not clean JSON: %q err=%v value=%#v", stdout.String(), err, value)
	}
	if strings.Contains(stdout.String(), "Executing command") || !strings.Contains(stderr.String(), "Executing command") || !strings.Contains(stderr.String(), "Loading machine output") {
		t.Fatalf("diagnostic routing stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
