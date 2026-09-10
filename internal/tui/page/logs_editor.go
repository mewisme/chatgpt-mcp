package page

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

func (page *LogsPage) initFilterEditor() {
	if page == nil || page.editor != nil {
		return
	}
	editor, data := newLogsFilterEditor(page.options, page.visibility)
	page.editor, page.filterForm = &editor, data
	page.resizeFilterEditor()
}

func (page *LogsPage) closeFilterEditor() tea.Cmd {
	if page == nil {
		return nil
	}
	page.editor, page.filterForm, page.action = nil, nil, ""
	return func() tea.Msg { return NavigateMsg{Path: []string{"logs"}, Replace: true, PreservePage: true} }
}

func (page *LogsPage) filterEditorView(width, height int) string {
	if page == nil || page.editor == nil {
		return ""
	}
	page.width, page.height = width, height
	title := component.PageTitle("Log Filters", width)
	page.resizeFilterEditor()
	return title + "\n" + page.editor.View()
}

func (page *LogsPage) resizeFilterEditor() {
	if page == nil || page.editor == nil || page.width <= 0 || page.height <= 0 {
		return
	}
	title := component.PageTitle("Log Filters", page.width)
	page.editor.Resize(page.width, max(1, page.height-lipgloss.Height(title)-1))
}

func (page *LogsPage) filterEditorMouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil || page.editor == nil {
		return nil
	}
	title := component.PageTitle("Log Filters", page.width)
	return page.editor.MouseTargets(originX, originY+lipgloss.Height(title)+1, z)
}
