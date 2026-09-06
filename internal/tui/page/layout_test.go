package page

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestBrowserPageHintsStayOnBottomRow(t *testing.T) {
	const width, height = 100, 24

	prepareConfigPageRoot(t)
	configPage, err := NewConfig(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := configPage.Update(configPage.Init()())
	configPage = updated.(*ConfigPage)
	assertPageBottomHint(t, "config", configPage.View(width, height), height, "? more")
	configPage.notice = "Config saved"
	assertPageBottomHint(t, "config with feedback", configPage.View(width, height), height, "? more")

	logsPage, err := NewLogs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer logsPage.Close()
	assertPageBottomHint(t, "logs runtime", logsPage.View(width, height), height, "? more")
	logsPage.notice = "Reconnecting"
	assertPageBottomHint(t, "logs runtime with feedback", logsPage.View(width, height), height, "? more")
	logsPage.notice = ""
	logsPage.tab = logsTabCommandExec
	assertPageBottomHint(t, "logs command execution", logsPage.View(width, height), height, "clear view")

	runtimePage, err := NewRuntime(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	runtimePage.loaded = true
	runtimePage.rebuildBrowser("")
	assertPageBottomHint(t, "runtime", runtimePage.View(width, height), height, "r refresh")
	runtimePage.notice = "Runtime updated"
	assertPageBottomHint(t, "runtime with feedback", runtimePage.View(width, height), height, "r refresh")
}

func assertPageBottomHint(t *testing.T, name, view string, height int, marker string) {
	t.Helper()
	lines := strings.Split(ansi.Strip(view), "\n")
	if len(lines) != height {
		t.Fatalf("%s height=%d want=%d view=%q", name, len(lines), height, ansi.Strip(view))
	}
	if !strings.Contains(lines[height-1], marker) {
		t.Fatalf("%s bottom row missing %q: %q", name, marker, lines[height-1])
	}
}
