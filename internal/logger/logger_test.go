package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDefaultRendererIsCLIFirst(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Info, Writer: &output})
	log.Ready("SERVER", "server.ready", "Server ready", With("mcp", "http://127.0.0.1:37421/mcp"), WithVerbose("bind", "127.0.0.1:37421"), WithDebug("listener_id", "listener-1"))
	text := output.String()
	for _, expected := range []string{"✓ Server ready", "mcp: http://127.0.0.1:37421/mcp"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
	for _, hidden := range []string{"INF", "SERVER", "bind", "listener_id", "server.ready"} {
		if strings.Contains(text, hidden) {
			t.Fatalf("default output %q unexpectedly contains %q", text, hidden)
		}
	}
}

func TestVerboseRendererAddsUsefulContext(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Info, Mode: ModeVerbose, Writer: &output})
	log.Ready("TUNNEL", "tunnel.connected", "Tunnel connected", WithVerbose("tunnel_id", "tunnel_123"), WithDebug("client_instance_id", "client_123"))
	text := output.String()
	if !strings.Contains(text, "tunnel_id: tunnel_123") || strings.Contains(text, "client_instance_id") {
		t.Fatalf("verbose output = %q", text)
	}
}

func TestDebugRendererIncludesStructuredMetadata(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Debug, Mode: ModeDebug, Writer: &output})
	log.now = func() time.Time { return time.Date(2026, 8, 31, 0, 27, 32, 0, time.UTC) }
	log.Diagnostic(Debug, "TUNNEL", "tunnel.route.resolved", "route resolved", WithDebug("client_instance_id", "client_123"), WithDebug("route_kind", "mcp_channel"))
	text := output.String()
	for _, expected := range []string{"00:27:32", "DBG", "TUNNEL", "tunnel.route.resolved", "client_instance_id=client_123", "route_kind=mcp_channel"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("debug output %q missing %q", text, expected)
		}
	}
}

func TestJSONRendererRespectsModeAndKeepsStructure(t *testing.T) {
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Info, Mode: ModeVerbose, Format: FormatJSON, Writer: &output})
	log.now = func() time.Time { return time.Date(2026, 8, 31, 0, 27, 32, 0, time.UTC) }
	log.Failure("TUNNEL", "tunnel.failed", "Tunnel failed", errors.New("dial failed"), With("tunnel_id", "tunnel_123"), WithVerbose("transport", "in-memory"), WithDebug("client_instance_id", "client_123"))
	var value map[string]any
	if err := json.Unmarshal(output.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value["event"] != "tunnel.failed" || value["level"] != "error" || value["kind"] != "error" || value["error"] != "dial failed" {
		t.Fatalf("json event = %#v", value)
	}
	fields, ok := value["fields"].(map[string]any)
	if !ok || fields["tunnel_id"] != "tunnel_123" || fields["transport"] != "in-memory" {
		t.Fatalf("json fields = %#v", value["fields"])
	}
	if _, ok := fields["client_instance_id"]; ok {
		t.Fatalf("debug field leaked into verbose json: %#v", fields)
	}
}

type captureSink struct{ events []Event }

func (s *captureSink) WriteEvent(event Event) error {
	s.events = append(s.events, event)
	return nil
}

func TestSinkReceivesNormalizedEventBeforeVisibilityFiltering(t *testing.T) {
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Info, Writer: &output})
	sink := &captureSink{}
	log.AddSink(sink)
	log.now = func() time.Time { return time.Date(2026, 8, 31, 1, 2, 3, 0, time.UTC) }
	log.Diagnostic(Debug, "TOOL", "tool.call.started", "Tool call started", WithDebug("tool", "run_command"))
	if output.Len() != 0 {
		t.Fatalf("debug event leaked into default output: %q", output.String())
	}
	if len(sink.events) != 1 {
		t.Fatalf("sink events = %#v", sink.events)
	}
	event := sink.events[0]
	if event.Time.IsZero() || event.Name != "tool.call.started" || event.Component != "TOOL" || event.Visibility != VisibilityDebug {
		t.Fatalf("sink event = %#v", event)
	}
}
