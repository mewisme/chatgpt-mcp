package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"

	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestLogTunnelLifecycleReconnect(t *testing.T) {
	previous := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = previous }()

	var output bytes.Buffer
	log := logger.NewWithOptions(logger.Options{Level: logger.Info, Mode: logger.ModeVerbose, Writer: &output})
	logTunnelLifecycle(log, tunnel.LifecycleEvent{State: tunnel.LifecycleReconnecting, ID: "tunnel_test", Attempt: 3, RetryIn: 4 * time.Second})
	text := output.String()
	for _, expected := range []string{"⠋ Reconnecting tunnel", "tunnel_id: tunnel_test", "attempt: 3", "retry_in: 4s"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
}

func TestTunnelCommandHierarchy(t *testing.T) {
	cmd := tunnelCommand()
	for _, path := range [][]string{{"admin", "list"}, {"admin", "add"}, {"admin", "verify"}, {"admin", "remove"}, {"managed", "list"}, {"managed", "get"}, {"managed", "create"}, {"managed", "update"}, {"managed", "delete"}, {"list"}, {"status"}, {"attach"}, {"detach"}, {"enable"}, {"disable"}, {"start"}, {"stop"}, {"run"}} {
		resolved, _, err := cmd.Find(path)
		if err != nil || resolved.Name() != path[len(path)-1] {
			t.Fatalf("tunnel path %v resolved to %v: %v", path, resolved, err)
		}
	}
	for _, legacy := range [][]string{{"use"}, {"select"}, {"switch"}, {"configure"}, {"admin", "key"}} {
		if resolved, remaining, err := cmd.Find(legacy); err == nil && len(remaining) == 0 && resolved.Name() == legacy[len(legacy)-1] {
			t.Fatalf("legacy tunnel path %v is still registered as %s", legacy, resolved.CommandPath())
		}
	}
}

func TestTunnelRunRequiresTunnelID(t *testing.T) {
	cmd := tunnelRunCommand()
	if err := cmd.Args(cmd, nil); err == nil {
		t.Fatal("tunnel run accepted a missing tunnel id")
	}
	if err := cmd.Args(cmd, []string{"tunnel_a"}); err != nil {
		t.Fatalf("tunnel run rejected one tunnel id: %v", err)
	}
}
