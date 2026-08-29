package logger

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"
)

func TestLineWriterStructuredLog(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()

	var output bytes.Buffer
	log := NewWithWriter(Debug, &output)
	log.timestamp = false
	writer := log.LineWriter("TUNNEL")

	_, err := writer.Write([]byte(`time=2026-08-29T12:00:00Z level=INFO msg="connected to control plane" tunnel_id=tunnel_123 api_key=secret` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"INF", "TUNNEL", "connected to control plane", "tunnel_id=tunnel_123", "api_key=[redacted]"} {
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
	log := NewWithWriter(Debug, &output)
	log.timestamp = false
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

func TestLineWriterHandlesMultipleAndRawLines(t *testing.T) {
	restoreColor := disableColor()
	defer restoreColor()

	var output bytes.Buffer
	log := NewWithWriter(Debug, &output)
	log.timestamp = false
	writer := log.LineWriter("TUNNEL")

	if _, err := writer.Write([]byte("first raw line\nlevel=ERROR msg=\"second line\" code=500\n")); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"INF", "first raw line", "ERR", "second line", "code=500"} {
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
