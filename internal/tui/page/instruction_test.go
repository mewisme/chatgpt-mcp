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
	for _, want := range []string{"Instruction Sources", "Claude · enabled", "Context · 1 · detected"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("sources view missing %q: %q", want, plain)
		}
	}
}

func TestInstructionPageManagesSourcePolicy(t *testing.T) {
	page, service := newTestInstructionPage(t)
	page.switchTab(instructionTabSources)
	page.sources.Down()
	updated, cmd := page.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	page = updated.(*InstructionPage)
	if cmd == nil || !page.saving {
		t.Fatalf("provider toggle cmd=%v saving=%t", cmd, page.saving)
	}
	updated, _ = page.Update(cmd())
	page = updated.(*InstructionPage)
	settings, err := service.Load()
	if err != nil {
		t.Fatal(err)
	}
	policy := settings.SourcePolicy["claude"]
	if policy.Enabled == nil || *policy.Enabled || page.Notice() != "Claude source disabled" {
		t.Fatalf("provider policy=%#v notice=%q", policy, page.Notice())
	}
	if len(settings.DetectedSources) != 1 || settings.DetectedSources[0].Enabled {
		t.Fatalf("disabled source=%#v", settings.DetectedSources)
	}

	page.sources.Down()
	page.sources.Down()
	updated, cmd = page.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	page = updated.(*InstructionPage)
	if cmd != nil || page.Notice() != "Enable the provider before changing resource policy" {
		t.Fatalf("disabled child cmd=%v notice=%q", cmd, page.Notice())
	}

	page.sources.GoToTop()
	page.sources.Down()
	updated, cmd = page.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	page = updated.(*InstructionPage)
	updated, _ = page.Update(cmd())
	page = updated.(*InstructionPage)
	settings, err = service.Load()
	if err != nil {
		t.Fatal(err)
	}
	policy = settings.SourcePolicy["claude"]
	if policy.Enabled == nil || !*policy.Enabled {
		t.Fatalf("provider was not re-enabled: %#v", policy)
	}

	page.sources.Down()
	page.sources.Down()
	updated, cmd = page.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	page = updated.(*InstructionPage)
	if cmd == nil {
		t.Fatal("resource toggle returned no command")
	}
	updated, _ = page.Update(cmd())
	page = updated.(*InstructionPage)
	settings, err = service.Load()
	if err != nil {
		t.Fatal(err)
	}
	policy = settings.SourcePolicy["claude"]
	if policy.Context == nil || *policy.Context || page.Notice() != "Claude Context disabled" {
		t.Fatalf("resource policy=%#v notice=%q", policy, page.Notice())
	}
}

func TestInstructionSourcesMouseWheelUsesSemanticMessage(t *testing.T) {
	page, _ := newTestInstructionPage(t)
	page.switchTab(instructionTabSources)
	page.View(100, 30)
	targets := page.sourceMouseTargets(2, 4, 10)
	if len(targets) != 1 || targets[0].ID != "instruction.sources.scroll" {
		t.Fatalf("targets=%#v", targets)
	}
	message, ok := targets[0].Handle(component.MouseEvent{Button: tea.MouseWheelDown}).(instructionSourceWheelMsg)
	if !ok || message != 1 {
		t.Fatalf("wheel message=%#v", message)
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

func TestInstructionPageManagesGlobalRules(t *testing.T) {
	page, service := newTestInstructionPage(t)
	updated, _ := page.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	page = updated.(*InstructionPage)
	if page.tab != instructionTabRules {
		t.Fatalf("tab=%d", page.tab)
	}

	updated, cmd := page.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	page = updated.(*InstructionPage)
	if cmd == nil || !page.saving {
		t.Fatalf("toggle cmd=%v saving=%t", cmd, page.saving)
	}
	updated, _ = page.Update(cmd())
	page = updated.(*InstructionPage)
	settings, err := service.Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Rules[0].Enabled || page.Notice() != "Global rule disabled" {
		t.Fatalf("toggle rule=%#v notice=%q", settings.Rules[0], page.Notice())
	}

	updated, initCmd := page.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	page = updated.(*InstructionPage)
	if !page.ruleFormActive || initCmd == nil || page.ruleEditID != "rule_one" {
		t.Fatalf("edit form active=%t init=%v id=%q", page.ruleFormActive, initCmd, page.ruleEditID)
	}
	page.ruleName, page.ruleContent, page.ruleEnabled = "Updated rule", "Updated content", true
	updated, cmd = page.Update(component.FormSubmittedMsg{})
	page = updated.(*InstructionPage)
	if cmd == nil {
		t.Fatal("edit submit returned no command")
	}
	updated, _ = page.Update(cmd())
	page = updated.(*InstructionPage)
	settings, err = service.Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Rules[0].Name != "Updated rule" || settings.Rules[0].Content != "Updated content" || !settings.Rules[0].Enabled {
		t.Fatalf("edited rule=%#v", settings.Rules[0])
	}

	updated, initCmd = page.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	page = updated.(*InstructionPage)
	if !page.ruleFormActive || initCmd == nil || !strings.HasPrefix(page.ruleEditID, "rule_") || page.ruleEditID == "rule_one" {
		t.Fatalf("add form active=%t init=%v id=%q", page.ruleFormActive, initCmd, page.ruleEditID)
	}
	createdID := page.ruleEditID
	page.ruleName, page.ruleContent, page.ruleEnabled = "Second rule", "Second content", true
	updated, cmd = page.Update(component.FormSubmittedMsg{})
	page = updated.(*InstructionPage)
	updated, _ = page.Update(cmd())
	page = updated.(*InstructionPage)
	settings, err = service.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Rules) != 2 || settings.Rules[1].ID != createdID || page.Notice() != "Global rule created" {
		t.Fatalf("created rules=%#v notice=%q", settings.Rules, page.Notice())
	}

	if !page.rules.SelectID(createdID) {
		t.Fatalf("created rule %q not selectable", createdID)
	}
	updated, _ = page.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	page = updated.(*InstructionPage)
	if page.ruleDeleteID != createdID || !page.OverlayActive() {
		t.Fatalf("delete id=%q overlay=%t", page.ruleDeleteID, page.OverlayActive())
	}
	updated, cmd = page.Update(component.ConfirmChoiceMsg{Affirmative: true})
	page = updated.(*InstructionPage)
	if cmd == nil || !page.saving {
		t.Fatalf("delete cmd=%v saving=%t", cmd, page.saving)
	}
	updated, _ = page.Update(cmd())
	page = updated.(*InstructionPage)
	settings, err = service.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Rules) != 1 || settings.Rules[0].ID != "rule_one" || page.OverlayActive() {
		t.Fatalf("deleted rules=%#v overlay=%t", settings.Rules, page.OverlayActive())
	}
}

func TestInstructionRuleBrowserSearchesRuleContent(t *testing.T) {
	page, _ := newTestInstructionPage(t)
	page.switchTab(instructionTabRules)
	page.rules.StartFilter()
	updated, _ := page.Update(tea.KeyPressMsg{Text: "verify", Code: 'v'})
	page = updated.(*InstructionPage)
	if !page.rules.InputActive() {
		t.Fatal("rule browser filter is not active")
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
