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
	for _, expected := range []string{"→ Reconnecting tunnel", "tunnel_id: tunnel_test", "attempt: 3", "retry_in: 4s"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
}
