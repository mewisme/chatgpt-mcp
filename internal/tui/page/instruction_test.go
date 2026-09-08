package page

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/instructionpolicy"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

func TestInstructionPageShowsContextAndReadOnlySummaries(t *testing.T) {
	page, _ := newTestInstructionPage(t)
	plain := ansi.Strip(page.View(100, 30))
	for _, want := range []string{"Context", "Rules", "Sources", "Global Context", "Shared instructions", "e edit", "r refresh"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("context view missing %q: %q", want, plain)
		}
	}

	updated, _ := page.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	page = updated.(*InstructionPage)
	plain = ansi.Strip(page.View(100, 30))
	for _, want := range []string{"Global Rules", "One global rule", "enabled", "rule_one"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rules view missing %q: %q", want, plain)
		}
	}

	updated, _ = page.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	page = updated.(*InstructionPage)
	plain = ansi.Strip(page.View(100, 30))
	for _, want := range []string{"Instruction Sources", "claude", "context"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("sources view missing %q: %q", want, plain)
		}
	}
}

func TestInstructionPageEditsAndSavesGlobalContext(t *testing.T) {
	page, service := newTestInstructionPage(t)
	updated, initCmd := page.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	page = updated.(*InstructionPage)
	if page.editor == nil || !page.InputActive() || initCmd == nil {
		t.Fatalf("editor active=%t input=%t init=%v", page.editor != nil, page.InputActive(), initCmd)
	}

	updated, saveCmd := page.Update(component.TextAreaSavedMsg{Value: "# Updated\n\nUse pnpm."})
	page = updated.(*InstructionPage)
	if saveCmd == nil || !page.saving {
		t.Fatalf("save cmd=%v saving=%t", saveCmd, page.saving)
	}
	updated, _ = page.Update(saveCmd())
	page = updated.(*InstructionPage)
	if page.editor != nil || page.saving || page.Notice() != "Global context saved" {
		t.Fatalf("saved editor=%v saving=%t notice=%q err=%v", page.editor != nil, page.saving, page.Notice(), page.err)
	}
	settings, err := service.Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Context != "# Updated\n\nUse pnpm." {
		t.Fatalf("stored context=%q", settings.Context)
	}
	if plain := ansi.Strip(page.View(100, 30)); !strings.Contains(plain, "Use pnpm.") {
		t.Fatalf("updated context not rendered: %q", plain)
	}
}

func TestInstructionPageRefreshesExternalChanges(t *testing.T) {
	page, service := newTestInstructionPage(t)
	value := "Externally updated"
	if _, err := service.Save(application.InstructionSettingsPatch{Context: &value}); err != nil {
		t.Fatal(err)
	}
	updated, cmd := page.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	page = updated.(*InstructionPage)
	if cmd == nil {
		t.Fatal("refresh command is nil")
	}
	updated, _ = page.Update(cmd())
	page = updated.(*InstructionPage)
	if page.settings.Context != value || page.Notice() != "Instructions refreshed" {
		t.Fatalf("context=%q notice=%q err=%v", page.settings.Context, page.Notice(), page.err)
	}
}

func TestInstructionPageMouseTabsUseSemanticMessage(t *testing.T) {
	page, _ := newTestInstructionPage(t)
	page.View(100, 30)
	targets := page.MouseTargets(2, 3, 10)
	var sourceTarget *component.MouseTarget
	for index := range targets {
		if targets[index].ID != "instruction.tab" {
			continue
		}
		message := targets[index].Handle(component.MouseEvent{Button: tea.MouseLeft})
		if tab, ok := message.(instructionTabMsg); ok && tab.Tab == instructionTabSources {
			sourceTarget = &targets[index]
			break
		}
	}
	if sourceTarget == nil {
		t.Fatal("sources tab target not found")
	}
	message := sourceTarget.Handle(component.MouseEvent{Button: tea.MouseLeft})
	updated, _ := page.Update(message)
	page = updated.(*InstructionPage)
	if page.tab != instructionTabSources {
		t.Fatalf("tab=%d", page.tab)
	}
}

func newTestInstructionPage(t *testing.T) (*InstructionPage, *application.InstructionSettingsService) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte("User context"), 0644); err != nil {
		t.Fatal(err)
	}
	store := &instructionpolicy.Store{Path: filepath.Join(t.TempDir(), "global.json")}
	value := instructionpolicy.DefaultConfig()
	value.Context = "# Shared instructions\n\nPrefer compact code."
	value.Rules = []instructionpolicy.GlobalRule{{ID: "rule_one", Name: "One global rule", Enabled: true, Content: "Always verify."}}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	service := application.NewInstructionSettingsService(store)
	service.UserHomeDir = func() (string, error) { return home, nil }
	page, err := newInstructionPage(t.Context(), service)
	if err != nil {
		t.Fatal(err)
	}
	return page, service
}
