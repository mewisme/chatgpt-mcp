package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
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

func TestTextRendererCapitalizesMessagesAfterIcons(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Info, Writer: &output})
	log.Ready("SERVICE", "service.updated", "managed service updated")
	log.Action("SERVICE", "service.updating", "updating managed service")
	log.Info("WORKSPACE", "registered workspaces loaded")
	text := output.String()
	for _, expected := range []string{"✓ Managed service updated", "→ Updating managed service", "· Registered workspaces loaded"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
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

func TestTimeModeShowsTimestampOnlyOnPrimaryTextLine(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Info, TimeMode: TimeShow, Writer: &output})
	log.now = func() time.Time { return time.Date(2026, 8, 31, 12, 34, 56, 0, time.UTC) }
	log.Ready("SERVER", "server.ready", "Server ready", With("mcp", "http://127.0.0.1:37421/mcp"))
	text := output.String()
	if !strings.Contains(text, "12:34:56 ✓ Server ready") {
		t.Fatalf("timestamped output = %q", text)
	}
	if strings.Contains(text, "12:34:56     mcp:") {
		t.Fatalf("detail line unexpectedly timestamped: %q", text)
	}
}

func TestTimeHideDisablesDebugTimestamp(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Debug, Mode: ModeDebug, TimeMode: TimeHide, Writer: &output})
	log.now = func() time.Time { return time.Date(2026, 8, 31, 12, 34, 56, 0, time.UTC) }
	log.Diagnostic(Debug, "TEST", "test.debug", "Debug line")
	if strings.Contains(output.String(), "12:34:56") {
		t.Fatalf("debug timestamp was not disabled: %q", output.String())
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
	for _, expected := range []string{"DBG", "TUNNEL", "tunnel.route.resolved", "client_instance_id=client_123", "route_kind=mcp_channel"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("debug output %q missing %q", text, expected)
		}
	}
	if strings.Contains(text, "00:27:32") {
		t.Fatalf("normal CLI debug output unexpectedly contains timestamp: %q", text)
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

func TestJSONRendererIncludesReplaySessionMetadata(t *testing.T) {
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Info, Format: FormatJSON, Writer: &output})
	log.Emit(Event{Level: Info, Kind: KindSuccess, Name: "server.ready", Message: "Server ready", Component: "SERVER", RunID: "run_test", PID: 42, Managed: true, ServiceID: "service_test", ServiceScope: "system"})
	var value map[string]any
	if err := json.Unmarshal(output.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value["run_id"] != "run_test" || value["pid"] != float64(42) || value["managed"] != true || value["service_id"] != "service_test" || value["service_scope"] != "system" {
		t.Fatalf("json metadata = %#v", value)
	}
}

func TestTextRendererUsesVisualHierarchyAndRespectsNoColor(t *testing.T) {
	previous := color.NoColor
	color.NoColor = false
	defer func() { color.NoColor = previous }()
	t.Setenv("NO_COLOR", "")
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Info, TimeMode: TimeShow, Writer: &output})
	log.now = func() time.Time { return time.Date(2026, 8, 31, 12, 34, 56, 0, time.UTC) }
	log.Ready("SERVER", "server.ready", "Server ready", With("mcp", "http://127.0.0.1:37421/mcp"))
	text := output.String()
	if !strings.Contains(text, "\x1b[") || !strings.Contains(text, "\x1b[2m") {
		t.Fatalf("styled output = %q", text)
	}
	if !strings.Contains(text, "Server ready") || !strings.Contains(text, "http://127.0.0.1:37421/mcp") {
		t.Fatalf("styled output lost content: %q", text)
	}

	t.Setenv("NO_COLOR", "1")
	output.Reset()
	log = NewWithOptions(Options{Level: Info, TimeMode: TimeShow, Writer: &output})
	log.now = func() time.Time { return time.Date(2026, 8, 31, 12, 34, 56, 0, time.UTC) }
	log.Ready("SERVER", "server.ready", "Server ready", With("mcp", "http://127.0.0.1:37421/mcp"))
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("NO_COLOR output contains ANSI: %q", output.String())
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
