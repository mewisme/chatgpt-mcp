package logger

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/fatih/color"
)

func TestLineWriterHidesDiagnosticsByDefault(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	writer := NewWithWriter(Info, &output).LineWriter("TUNNEL")
	_, err := writer.Write([]byte(`time=2026-08-29T12:00:00Z level=INFO msg="connected to control plane" tunnel_id=tunnel_123 api_key=secret` + "\nraw line\n"))
	if err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("default output leaked diagnostics: %q", output.String())
	}
}

func TestLineWriterDebugPreservesMetadataAndRedactsSecrets(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Debug, Mode: ModeDebug, Writer: &output})
	writer := log.LineWriter("TUNNEL")
	_, err := writer.Write([]byte(`time=2026-08-29T12:00:00Z level=INFO msg="connected to control plane" component=controlplane tunnel_id=tunnel_123 api_key=secret route_kind=mcp_channel` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"INF", "CONTROLPLANE", "connected to control plane", "tunnel_id=tunnel_123", "api_key=[redacted]", "route_kind=mcp_channel", "stream_component=TUNNEL"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
	if strings.Contains(text, "api_key=secret") {
		t.Fatalf("secret leaked in output: %q", text)
	}
}

func TestLineWriterBuffersFragmentsAndMapsLevel(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Debug, Mode: ModeDebug, Writer: &output})
	writer := log.LineWriter("TUNNEL")
	if _, err := writer.Write([]byte(`level=WARN msg="control plane `)); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("fragment emitted before newline: %q", output.String())
	}
	if _, err := writer.Write([]byte(`retry" attempt=2` + "\n")); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"WRN", "TUNNEL", "control plane retry", "attempt=2"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
}

func disableColor() func() {
	previous := color.NoColor
	color.NoColor = true
	return func() { color.NoColor = previous }
}

func TestLineWriterVerboseShowsRouteContextOnly(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Info, Mode: ModeVerbose, Writer: &output})
	writer := log.LineWriter("TUNNEL")
	_, err := writer.Write([]byte(`level=INFO msg="mcp channel route resolved" client_instance_id=client_123 tunnel_id=tunnel_123 component=mcpclient channel=main transport=in-memory mtls_enabled=false route_kind=mcp_channel route_name=main proxy_source=none` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"Mcp channel route resolved", "tunnel_id: tunnel_123", "channel: main", "transport: in-memory", "route_kind: mcp_channel", "route_name: main"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("verbose output %q missing %q", text, expected)
		}
	}
	for _, hidden := range []string{"client_instance_id", "mtls_enabled", "proxy_source", "component="} {
		if strings.Contains(text, hidden) {
			t.Fatalf("verbose output %q unexpectedly contains %q", text, hidden)
		}
	}
}

func TestLineWriterKeepsRawReconnectDiagnosticOutOfDefault(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	writer := NewWithWriter(Info, &output).LineWriter("TUNNEL")
	if _, err := writer.Write([]byte(`level=INFO msg="reconnecting to control plane" attempt=2 tunnel_id=tunnel_123` + "\n")); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("raw reconnect diagnostic leaked into default output: %q", output.String())
	}
}

func TestLineWriterRedactsSensitiveRawTokens(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Debug, Mode: ModeDebug, Writer: &output})
	writer := log.LineWriter("TUNNEL")
	if _, err := writer.Write([]byte("raw diagnostic api_key=secret token=hidden value=ok\n")); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Contains(text, "api_key=secret") || strings.Contains(text, "token=hidden") || !strings.Contains(text, "api_key=[redacted]") || !strings.Contains(text, "token=[redacted]") {
		t.Fatalf("raw diagnostic redaction = %q", text)
	}
}

func TestLineWriterNilAndDefaultComponent(t *testing.T) {
	var nilLog *Logger
	if nilLog.LineWriter("TEST") != io.Discard {
		t.Fatal("nil logger did not return io.Discard")
	}
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Debug, Mode: ModeDebug, Writer: &output})
	writer := log.LineWriter("   ")
	if _, err := writer.Write([]byte("level=INFO msg=hello\n\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "LOG") || !strings.Contains(output.String(), "hello") {
		t.Fatalf("default component output=%q", output.String())
	}
}

func TestLineWriterFlushesOversizedFragment(t *testing.T) {
	var output bytes.Buffer
	log := NewWithOptions(Options{Level: Debug, Mode: ModeDebug, Writer: &output})
	writer := log.LineWriter("RAW")
	line := strings.Repeat("x", maxBufferedLogLine+1)
	if n, err := writer.Write([]byte(line)); err != nil || n != len(line) {
		t.Fatalf("write n=%d err=%v", n, err)
	}
	if !strings.Contains(output.String(), "diagnostic.raw") || !strings.Contains(output.String(), "RAW") {
		t.Fatalf("oversized output=%q", output.String()[:min(output.Len(), 200)])
	}
}

func TestStructuredLineParsesMetadataErrorsAndInvalidTime(t *testing.T) {
	event, ok := parseStructuredLine(`time=bad level=ERROR event=test.failed component=INNER msg="request failed" error="secret failure" source=remote tunnel_id=tunnel_1`)
	if !ok || event.Level != Error || event.Kind != KindError || event.Name != "test.failed" || event.Component != "INNER" || event.Message != "request failed" || event.Err == nil || event.Err.Error() != "secret failure" {
		t.Fatalf("event=%#v ok=%t", event, ok)
	}
	keys := map[string]Visibility{}
	for _, field := range event.Fields {
		keys[field.Key] = field.Visibility
	}
	if keys["time"] != VisibilityDebug || keys["source"] != VisibilityDebug || keys["tunnel_id"] != VisibilityVerbose {
		t.Fatalf("fields=%#v", event.Fields)
	}
	if _, ok := parseStructuredLine(`level=INFO event=no-message`); ok {
		t.Fatal("structured line without message accepted")
	}
	if _, ok := parseStructuredLine(`key=value raw`); ok {
		t.Fatal("unrecognized structured tokens accepted")
	}
}

func TestStructuredTokenizerAndLevelAliases(t *testing.T) {
	tokens := splitStructuredTokens(`msg="hello \"quoted\" world" level=DBG key=value`)
	if len(tokens) != 3 || decodeStructuredValue(strings.TrimPrefix(tokens[0], "msg=")) != `hello "quoted" world` {
		t.Fatalf("tokens=%#v", tokens)
	}
	for input, want := range map[string]Level{"debug": Debug, "DBG": Debug, "warn": Warn, "WARNING": Warn, "WRN": Warn, "error": Error, "ERR": Error, "info": Info, "other": Info} {
		if got := parseStructuredLevel(input); got != want {
			t.Fatalf("parseStructuredLevel(%q)=%v want %v", input, got, want)
		}
	}
}
