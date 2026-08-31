package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"
)

func TestCLIVisualHierarchyAndNoColor(t *testing.T) {
	previous := color.NoColor
	color.NoColor = false
	defer func() { color.NoColor = previous }()
	t.Setenv("NO_COLOR", "")
	var output bytes.Buffer
	statusField(&output, "session", "run_test")
	statusStateField(&output, "status", "connected")
	text := output.String()
	if !strings.Contains(text, "\x1b[2m") || !strings.Contains(text, "\x1b[92;1mconnected") || !strings.Contains(text, "run_test") {
		t.Fatalf("styled status = %q", text)
	}

	t.Setenv("NO_COLOR", "1")
	output.Reset()
	statusField(&output, "session", "run_test")
	statusStateField(&output, "status", "connected")
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("NO_COLOR status contains ANSI: %q", output.String())
	}
}
