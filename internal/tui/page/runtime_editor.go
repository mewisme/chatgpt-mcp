package page

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

func (page *RuntimePage) initRuntimeEditor() error {
	if page == nil || page.action == "" || page.editor != nil {
		return nil
	}
	page.err, page.notice = nil, ""
	switch strings.TrimSpace(page.action) {
	case "install":
		editor, data := newInstallEditor()
		page.pending, page.editor, page.installForm = InstallRun, &editor, data
	case "update":
		editor, data := newUpdateEditor()
		page.pending, page.editor, page.updateForm = UpdateApply, &editor, data
	default:
		return nil
	}
	page.resizeRuntimeEditor()
	return nil
}

func (page *RuntimePage) submitRuntimeEditor() tea.Cmd {
	if page == nil || page.editor == nil || page.editor.Submitting() {
		return nil
	}
	if err := page.editor.Validate(); err != nil {
		page.editor.SetFeedback("", err)
		return nil
	}
	page.editor.SetSubmitting(true)
	return page.startOperation(page.pending)
}

func (page *RuntimePage) runtimeEditorParentNavigation() tea.Cmd {
	return func() tea.Msg { return NavigateMsg{Path: []string{"runtime"}, Replace: true} }
}

func (page *RuntimePage) runtimeEditorTitle() string {
	switch page.pending {
	case InstallRun:
		return "Managed Install"
	case UpdateApply:
		return "Apply Update"
	default:
		return "Runtime Editor"
	}
}

func (page *RuntimePage) runtimeEditorView(width, height int) string {
	if page == nil || page.editor == nil {
		return ""
	}
	page.width, page.height = width, height
	title := component.PageTitle(page.runtimeEditorTitle(), width)
	page.resizeRuntimeEditor()
	return title + "\n" + page.editor.View()
}

func (page *RuntimePage) resizeRuntimeEditor() {
	if page == nil || page.editor == nil || page.width <= 0 || page.height <= 0 {
		return
	}
	title := component.PageTitle(page.runtimeEditorTitle(), page.width)
	page.editor.Resize(page.width, max(1, page.height-lipgloss.Height(title)-1))
}

func (page *RuntimePage) runtimeEditorMouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil || page.editor == nil {
		return nil
	}
	title := component.PageTitle(page.runtimeEditorTitle(), page.width)
	return page.editor.MouseTargets(originX, originY+lipgloss.Height(title)+1, z)
}
