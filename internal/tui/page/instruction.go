package page

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type instructionTab uint8

const (
	instructionTabContext instructionTab = iota
	instructionTabRules
	instructionTabSources
)

var instructionTabLabels = []string{"Context", "Rules", "Sources"}

type instructionRefreshMsg struct {
	settings application.InstructionSettings
	err      error
}

type instructionSavedMsg struct {
	settings application.InstructionSettings
	err      error
}

type instructionTabMsg struct{ Tab instructionTab }

type InstructionPage struct {
	ctx            context.Context
	service        *application.InstructionSettingsService
	settings       application.InstructionSettings
	tab            instructionTab
	detail         component.DetailPage
	editor         *component.TextAreaEditor
	rules          component.Browser
	ruleForm       component.Form
	ruleFormActive bool
	ruleEditID     string
	ruleName       string
	ruleContent    string
	ruleEnabled    bool
	ruleDeleteID   string
	ruleConfirm    component.ConfirmButtons
	saving         bool
	notice         string
	err            error
	width          int
	height         int
}

func NewInstruction(ctx context.Context) (*InstructionPage, error) {
	return newInstructionPage(ctx, application.NewInstructionSettingsService(nil))
}

func newInstructionPage(ctx context.Context, service *application.InstructionSettingsService) (*InstructionPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if service == nil {
		service = application.NewInstructionSettingsService(nil)
	}
	settings, err := service.Load()
	if err != nil {
		return nil, err
	}
	page := &InstructionPage{ctx: ctx, service: service, settings: settings}
	page.syncDetail()
	return page, nil
}

func (page *InstructionPage) Init() tea.Cmd       { return nil }
func (page *InstructionPage) OverlayActive() bool { return page != nil && page.ruleDeleteID != "" }
func (page *InstructionPage) InputActive() bool {
	return page != nil && (page.editor != nil || page.ruleFormActive || page.tab == instructionTabRules && page.rules.InputActive())
}
func (page *InstructionPage) Notice() string {
	if page == nil {
		return ""
	}
	return page.notice
}
func (page *InstructionPage) SetNotice(value string) {
	if page != nil {
		page.notice = strings.TrimSpace(value)
	}
}

func (page *InstructionPage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		page.width, page.height = msg.Width, msg.Height
		page.resizeContent()
		if page.ruleFormActive {
			form, cmd := page.ruleForm.Update(msg)
			page.ruleForm = form
			return page, cmd
		}
		return page, nil
	case tea.BackgroundColorMsg:
		if page.editor != nil {
			updated, cmd := page.editor.Update(msg)
			page.editor = &updated
			return page, cmd
		}
		if page.ruleFormActive {
			form, cmd := page.ruleForm.Update(msg)
			page.ruleForm = form
			return page, cmd
		}
		if page.tab == instructionTabRules {
			updated, cmd := page.rules.Update(msg)
			page.rules = updated.(component.Browser)
			return page, cmd
		}
		updated, cmd := page.detail.Update(msg)
		page.detail = updated
		return page, cmd
	case component.FormSubmittedMsg:
		if page.ruleFormActive && !page.saving {
			page.saving = true
			page.err, page.notice = nil, ""
			return page, page.saveRuleFormCmd()
		}
		return page, nil
	case component.FormCancelledMsg:
		if page.ruleFormActive && !page.saving {
			page.closeRuleForm()
		}
		return page, nil
	case component.FormMouseMsg:
		if page.ruleFormActive && !page.saving {
			form, cmd := page.ruleForm.Update(msg)
			page.ruleForm = form
			return page, cmd
		}
		return page, nil
	case component.BrowserOpenMsg:
		if page.tab == instructionTabRules && !page.ruleFormActive && msg.Row.ID != "" {
			return page, page.openRuleForm(msg.Row.ID)
		}
		return page, nil
	case component.ConfirmChoiceMsg:
		if page.ruleDeleteID != "" {
			page.ruleConfirm.Select(msg.Affirmative)
			return page, page.updateRuleConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		return page, nil
	case component.TextAreaSavedMsg:
		if page.editor == nil || page.saving {
			return page, nil
		}
		page.saving = true
		page.err, page.notice = nil, ""
		return page, page.saveContextCmd(msg.Value)
	case component.TextAreaCancelledMsg:
		if !page.saving {
			page.editor = nil
			page.resizeContent()
		}
		return page, nil
	case instructionRefreshMsg:
		if msg.err != nil {
			page.err = msg.err
			return page, nil
		}
		page.settings, page.err = msg.settings, nil
		page.notice = "Instructions refreshed"
		page.syncDetail()
		return page, nil
	case instructionSavedMsg:
		page.saving = false
		if msg.err != nil {
			page.err = msg.err
			return page, nil
		}
		page.settings, page.err = msg.settings, nil
		page.notice = "Global context saved"
		page.editor = nil
		page.syncDetail()
		return page, nil
	case instructionRulesSavedMsg:
		page.saving = false
		if msg.err != nil {
			page.err = msg.err
			return page, nil
		}
		page.settings, page.err = msg.settings, nil
		page.notice = msg.notice
		page.closeRuleForm()
		page.closeRuleConfirm()
		page.syncRuleBrowser()
		return page, nil
	case instructionTabMsg:
		page.switchTab(msg.Tab)
		return page, nil
	case tea.KeyPressMsg:
		if page.ruleDeleteID != "" {
			return page, page.updateRuleConfirm(msg)
		}
		if page.editor != nil {
			if page.saving {
				return page, nil
			}
			updated, cmd := page.editor.Update(msg)
			page.editor = &updated
			return page, cmd
		}
		if page.ruleFormActive {
			if page.saving {
				return page, nil
			}
			form, cmd := page.ruleForm.Update(msg)
			page.ruleForm = form
			return page, cmd
		}
		if page.tab == instructionTabRules && page.rules.InputActive() {
			updated, cmd := page.rules.Update(msg)
			page.rules = updated.(component.Browser)
			return page, cmd
		}
		if cmd, handled := page.handleTabKey(msg); handled {
			return page, cmd
		}
		if page.tab == instructionTabRules {
			if cmd, handled := page.handleRuleKey(msg); handled {
				return page, cmd
			}
		}
		switch msg.String() {
		case "e":
			if page.tab == instructionTabContext {
				editor := component.NewTextAreaEditor("Global context", page.settings.Context)
				page.editor = &editor
				page.err, page.notice = nil, ""
				page.resizeContent()
				return page, page.editor.Init()
			}
		case "r":
			page.err, page.notice = nil, ""
			return page, page.refreshCmd()
		}
	}
	if page.editor != nil {
		updated, cmd := page.editor.Update(message)
		page.editor = &updated
		return page, cmd
	}
	if page.ruleFormActive {
		form, cmd := page.ruleForm.Update(message)
		page.ruleForm = form
		return page, cmd
	}
	if page.tab == instructionTabRules {
		updated, cmd := page.rules.Update(message)
		page.rules = updated.(component.Browser)
		return page, cmd
	}
	updated, cmd := page.detail.Update(message)
	page.detail = updated
	return page, cmd
}

func (page *InstructionPage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "Instruction page unavailable", "")
	}
	page.width, page.height = width, height
	tabs := component.PageTabsNotice(instructionTabLabels, int(page.tab), page.notice, width)
	bodyHeight := max(1, height-lipgloss.Height(tabs)-1)
	if page.tab == instructionTabRules {
		return page.rulesView(tabs, width, bodyHeight)
	}
	if page.editor != nil {
		feedback := ""
		if page.err != nil {
			feedback = component.BannerWidth(page.err.Error(), component.ToneDanger, width)
		}
		if feedback != "" {
			bodyHeight = max(1, bodyHeight-lipgloss.Height(feedback)-1)
		}
		page.editor.Resize(width, bodyHeight)
		body := page.editor.View()
		if feedback != "" {
			body = feedback + "\n" + body
		}
		return tabs + "\n" + body
	}
	page.detail.SetFeedback("", page.err)
	page.detail.Resize(width, bodyHeight)
	return tabs + "\n" + page.detail.View()
}

func (page *InstructionPage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	tabs, spans := component.PageTabsLayout(instructionTabLabels, int(page.tab), page.notice, page.width)
	if page.ruleDeleteID != "" {
		return page.ruleConfirmMouseTargets(originX, originY, z)
	}
	targets := make([]component.MouseTarget, 0, len(spans)+1)
	for _, span := range spans {
		tab := instructionTab(span.Index)
		targets = append(targets, component.MouseTarget{
			ID: "instruction.tab", Rect: component.Rect{X: originX + span.X, Y: originY, Width: span.Width, Height: 1}, Z: z + 2,
			Handle: func(event component.MouseEvent) tea.Msg {
				if event.Button != tea.MouseLeft {
					return nil
				}
				return instructionTabMsg{Tab: tab}
			},
		})
	}
	contentY := originY + lipgloss.Height(tabs) + 1
	if page.tab == instructionTabRules {
		if page.ruleFormActive {
			return append(targets, page.ruleForm.MouseTargets(originX, contentY, z)...)
		}
		return append(targets, page.rules.MouseTargets(originX, contentY, z)...)
	}
	if page.editor == nil {
		targets = append(targets, page.detail.MouseTargets(originX, contentY, z)...)
	}
	return targets
}

func (page *InstructionPage) handleTabKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "1":
		page.switchTab(instructionTabContext)
		return nil, true
	case "2":
		page.switchTab(instructionTabRules)
		return nil, true
	case "3":
		page.switchTab(instructionTabSources)
		return nil, true
	}
	delta, ok := component.TabDelta(msg)
	if !ok {
		return nil, false
	}
	page.switchTab(instructionTab(component.MoveTab(int(page.tab), len(instructionTabLabels), delta)))
	return nil, true
}

func (page *InstructionPage) switchTab(tab instructionTab) {
	if page == nil || int(tab) < 0 || int(tab) >= len(instructionTabLabels) || page.tab == tab {
		return
	}
	page.tab = tab
	page.err, page.notice = nil, ""
	page.syncDetail()
}

func (page *InstructionPage) syncDetail() {
	if page == nil {
		return
	}
	switch page.tab {
	case instructionTabContext:
		content := page.settings.Context
		meta := formatInstructionBytes(len([]byte(content)))
		if strings.TrimSpace(content) == "" {
			content, meta = component.Muted("No managed global context."), "not configured"
		}
		page.detail = component.NewDetailPage("Global Context", meta, content)
		page.detail.SetBindings(
			component.DetailPageBinding{Key: "e", Desc: "edit", Message: tea.KeyPressMsg{Code: 'e', Text: "e"}},
			component.DetailPageBinding{Key: "r", Desc: "refresh", Message: tea.KeyPressMsg{Code: 'r', Text: "r"}},
		)
	case instructionTabRules:
		page.syncRuleBrowser()
	case instructionTabSources:
		lines := make([]string, 0)
		for _, source := range page.settings.DetectedSources {
			state := "detected"
			if !source.Enabled {
				state = "disabled"
			} else if source.Loaded {
				state = "included"
			}
			lines = append(lines, fmt.Sprintf("%s · %s · %d · %s", source.Provider, source.Kind, source.Count, state))
			for _, path := range source.Paths {
				lines = append(lines, "  "+path)
			}
		}
		page.detail = component.NewDetailPage("Instruction Sources", fmt.Sprintf("%d detected", len(page.settings.DetectedSources)), detailList(lines))
		page.detail.SetBindings(component.DetailPageBinding{Key: "r", Desc: "refresh", Message: tea.KeyPressMsg{Code: 'r', Text: "r"}})
	}
	page.resizeContent()
}

func (page *InstructionPage) resizeContent() {
	if page == nil || page.width <= 0 || page.height <= 0 {
		return
	}
	tabs := component.PageTabsNotice(instructionTabLabels, int(page.tab), page.notice, page.width)
	height := max(1, page.height-lipgloss.Height(tabs)-1)
	if page.editor != nil {
		page.editor.Resize(page.width, height)
		return
	}
	if page.tab == instructionTabRules {
		if page.ruleFormActive {
			form, _ := page.ruleForm.Update(tea.WindowSizeMsg{Width: page.width, Height: height})
			page.ruleForm = form
			return
		}
		updated, _ := page.rules.Update(tea.WindowSizeMsg{Width: page.width, Height: height})
		page.rules = updated.(component.Browser)
		return
	}
	page.detail.Resize(page.width, height)
}

func (page *InstructionPage) refreshCmd() tea.Cmd {
	service := page.service
	return func() tea.Msg {
		settings, err := service.Load()
		return instructionRefreshMsg{settings: settings, err: err}
	}
}

func (page *InstructionPage) saveContextCmd(value string) tea.Cmd {
	service := page.service
	return func() tea.Msg {
		settings, err := service.Save(application.InstructionSettingsPatch{Context: &value})
		return instructionSavedMsg{settings: settings, err: err}
	}
}

func formatInstructionBytes(value int) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	if value < 10240 {
		return fmt.Sprintf("%.1f KB", float64(value)/1024)
	}
	return fmt.Sprintf("%.0f KB", float64(value)/1024)
}
