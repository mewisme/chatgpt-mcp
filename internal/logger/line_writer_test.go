package logger

import (
	"bytes"
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
	for _, expected := range []string{"12:00:00", "INF", "CONTROLPLANE", "connected to control plane", "tunnel_id=tunnel_123", "api_key=[redacted]", "route_kind=mcp_channel", "stream_component=TUNNEL"} {
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
	for _, expected := range []string{"mcp channel route resolved", "tunnel_id: tunnel_123", "channel: main", "transport: in-memory", "route_kind: mcp_channel", "route_name: main"} {
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

func TestLineWriterPromotesReconnectToDefaultAction(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()
	var output bytes.Buffer
	writer := NewWithWriter(Info, &output).LineWriter("TUNNEL")
	if _, err := writer.Write([]byte(`level=INFO msg="reconnecting to control plane" attempt=2 tunnel_id=tunnel_123` + "\n")); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "→ Reconnecting tunnel") || strings.Contains(text, "attempt") || strings.Contains(text, "tunnel_id") {
		t.Fatalf("default reconnect output = %q", text)
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
