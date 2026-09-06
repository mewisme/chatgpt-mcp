package component

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCharmThemeSemanticStylesDoNotInjectComponentGlyphs(t *testing.T) {
	SetDarkBackground(true)
	for name, value := range map[string]string{
		"accent":  ToneText("> ", ToneAccent),
		"success": ToneText("[x]", ToneSuccess),
		"danger":  ToneText("error", ToneDanger),
	} {
		want := map[string]string{"accent": "> ", "success": "[x]", "danger": "error"}[name]
		if got := ansi.Strip(value); got != want {
			t.Fatalf("%s rendered %q, want %q", name, got, want)
		}
	}
}

func TestPageTitleMatchesDefaultListTitleBar(t *testing.T) {
	SetDarkBackground(true)
	browser := NewBrowser(context.Background(), "Workspaces", []Row{{ID: "one", Title: "One"}}, nil)
	browser = updateBrowser(t, browser, tea.WindowSizeMsg{Width: 80, Height: 20})
	wantLines := strings.Split(ansi.Strip(browser.Content()), "\n")
	gotLines := strings.Split(ansi.Strip(PageTitle("Workspaces", 80)), "\n")
	if len(wantLines) < 2 || len(gotLines) < 2 || strings.TrimRight(gotLines[0], " ") != strings.TrimRight(wantLines[0], " ") || strings.TrimSpace(gotLines[1]) != "" || strings.TrimSpace(wantLines[1]) != "" {
		t.Fatalf("page title=%q list title=%q", gotLines, wantLines[:min(2, len(wantLines))])
	}
}
