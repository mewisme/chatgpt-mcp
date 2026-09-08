package component

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"go.mewis.me/chatgpt-mcp/internal/tui/testutil"
)

func TestEditorMovesAcrossVisibleSectionsWithoutCompletingForm(t *testing.T) {
	general, detail := "general", "detail"
	editor := NewEditor("save",
		EditorSection{ID: "general", Title: "General", Form: NewEditorForm(Group(Input("General", &general)))},
		EditorSection{ID: "detail", Title: "Detail", Form: NewEditorForm(Group(Input("Detail", &detail)))},
	)
	editor = runEditorCmd(t, editor, editor.Init())
	updated, cmd := editor.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	editor = runEditorCmd(t, updated, cmd)
	if editor.ActiveSectionID() != "detail" {
		t.Fatalf("active section=%q", editor.ActiveSectionID())
	}
	form, _ := editor.SectionForm(0)
	if form.State() != huh.StateNormal {
		t.Fatalf("first form state=%v", form.State())
	}
	updated, cmd = editor.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	editor = runEditorCmd(t, updated, cmd)
	if editor.ActiveSectionID() != "general" {
		t.Fatalf("back section=%q", editor.ActiveSectionID())
	}
}

func TestEditorSubmitCancelFeedbackDirtyAndResponsiveLayout(t *testing.T) {
	value := "demo"
	editor := NewEditor("create", EditorSection{ID: "main", Title: "Main", Description: strings.Repeat("description ", 12), Form: NewEditorForm(Group(Input("Name", &value)))})
	editor = runEditorCmd(t, editor, editor.Init())
	editor.Resize(24, 10)
	testutil.AssertLinesFit(t, editor.View(), 24)
	value = "changed"
	if !editor.Dirty() {
		t.Fatal("editor did not detect dirty draft")
	}
	_, cmd := editor.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+s returned no submit command")
	}
	if _, ok := cmd().(EditorSubmitMsg); !ok {
		t.Fatalf("submit message=%T", cmd())
	}
	_, cmd = editor.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc returned no cancel command")
	}
	if _, ok := cmd().(EditorCancelMsg); !ok {
		t.Fatalf("cancel message=%T", cmd())
	}
	editor.SetFeedback("saved", errors.New("invalid "+strings.Repeat("value ", 10)))
	testutil.AssertLinesFit(t, editor.View(), 24)
	editor.SetSubmitting(true)
	_, cmd = editor.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("submitting editor accepted second submit")
	}
}

func TestEditorMouseSectionTargetsUseSharedGeometry(t *testing.T) {
	one, two := "one", "two"
	editor := NewEditor("save",
		EditorSection{ID: "one", Title: "One", Form: NewEditorForm(Group(Input("One", &one)))},
		EditorSection{ID: "two", Title: "Two", Form: NewEditorForm(Group(Input("Two", &two)))},
	)
	editor.Resize(40, 14)
	_ = editor.View()
	for _, target := range editor.MouseTargets(3, 2, 5) {
		if target.ID != "editor.section" {
			continue
		}
		message := target.Handle(MouseEvent{Button: tea.MouseLeft})
		section, ok := message.(EditorSectionMsg)
		if ok && section.Index == 1 {
			updated, _ := editor.Update(section)
			if updated.ActiveSection() != 1 {
				t.Fatalf("mouse section active=%d", updated.ActiveSection())
			}
			return
		}
	}
	t.Fatal("second editor section mouse target not found")
}

func runEditorCmd(t *testing.T, editor Editor, cmd tea.Cmd) Editor {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; steps < 64 && len(queue) > 0; steps++ {
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		message := next()
		if batch, ok := message.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		updated, followup := editor.Update(message)
		editor = updated
		if followup != nil {
			queue = append(queue, followup)
		}
	}
	return editor
}
