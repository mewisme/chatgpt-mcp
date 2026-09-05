package interactive

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type Row struct {
	ID          string
	Title       string
	Description string
	Meta        string
	Summary     string
	Detail      string
	DetailTitle string
	DetailRows  []Row
	DetailTabs  []DetailTab
	Search      string
}

type RefreshFunc func(context.Context) ([]Row, error)

type RowAction struct {
	Key  string
	Desc string
	Run  func(Row) (string, tea.Cmd, error)
}

type browserItem struct{ Row }

func (i browserItem) Title() string {
	if value := strings.TrimSpace(i.Row.Title); value != "" {
		return value
	}
	if value := strings.TrimSpace(i.Row.ID); value != "" {
		return value
	}
	return i.Row.Summary
}

func (i browserItem) Description() string {
	parts := make([]string, 0, 2)
	if value := strings.TrimSpace(i.Row.Description); value != "" {
		parts = append(parts, value)
	}
	if value := strings.TrimSpace(i.Row.Meta); value != "" {
		parts = append(parts, value)
	}
	return strings.Join(parts, " · ")
}

func (i browserItem) FilterValue() string {
	return strings.Join([]string{i.Row.ID, i.Row.Title, i.Row.Description, i.Row.Meta, i.Row.Summary, i.Row.Search}, " ")
}

type Browser struct {
	ctx                context.Context
	list               list.Model
	viewport           viewport.Model
	refresh            RefreshFunc
	actions            []RowAction
	detail             bool
	loading            bool
	width              int
	height             int
	err                error
	notice             string
	pendingSelectionID string
	detailTab          int
}

type browserRefreshMsg struct {
	rows []Row
	err  error
}

var browserOpenBinding = Binding([]string{"enter", "v"}, "enter", "details")
var browserRefreshBinding = Binding([]string{"r"}, "r", "refresh")

func NewBrowser(ctx context.Context, title string, rows []Row, refresh RefreshFunc) Browser {
	if ctx == nil {
		ctx = context.Background()
	}
	model := NewDefaultList(strings.TrimSpace(title), browserListItems(rows), 80, 20, "item", "items")
	model.SetShowStatusBar(len(rows) > 0)
	view := viewport.New(viewport.WithWidth(74), viewport.WithHeight(12))
	view.SoftWrap = true
	view.FillHeight = false
	result := Browser{ctx: ctx, list: model, viewport: view, refresh: refresh}
	result.syncHelp()
	return result
}

func (m Browser) WithAction(action RowAction) Browser {
	action.Key, action.Desc = strings.TrimSpace(action.Key), strings.TrimSpace(action.Desc)
	if action.Key != "" && action.Run != nil {
		m.actions = append(m.actions, action)
		m.syncHelp()
	}
	return m
}

func (m Browser) Init() tea.Cmd { return nil }

func (m Browser) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		ApplyDefaultListTheme(&m.list, msg.IsDark())
		m.syncHelp()
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		ResizeDefaultList(&m.list, msg.Width, msg.Height)
		m.resizeViewport()
		return m, nil
	case browserRefreshMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			cmd := m.list.NewStatusMessage("Refresh failed: " + msg.err.Error())
			return m, cmd
		}
		m.err = nil
		selectedID := ""
		if selected, ok := m.selected(); ok {
			selectedID = selected.ID
		}
		m.list.SetShowStatusBar(len(msg.rows) > 0)
		cmd := m.list.SetItems(browserListItems(msg.rows))
		if cmd != nil {
			m.pendingSelectionID = selectedID
		} else {
			m.restoreSelection(selectedID)
		}
		if m.detail {
			if selected, ok := m.rowByID(selectedID); ok {
				m.syncDetail(selected)
			} else {
				m.detail = false
			}
		}
		return m, cmd
	case list.FilterMatchesMsg:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		if m.pendingSelectionID != "" {
			m.restoreSelection(m.pendingSelectionID)
			m.pendingSelectionID = ""
		}
		return m, cmd
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(message)
	return m, cmd
}

func (m Browser) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.detail {
		switch {
		case msg.String() == "ctrl+c":
			return m, tea.Quit
		case msg.String() == "q", msg.String() == "esc", key.Matches(msg, browserOpenBinding):
			m.detail = false
			return m, nil
		case m.moveDetailTab(msg):
			return m, nil
		case m.refresh != nil && key.Matches(msg, browserRefreshBinding):
			return m.startRefresh()
		default:
			if handled, cmd := m.runAction(msg.String()); handled {
				return m, cmd
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}
	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	switch {
	case key.Matches(msg, browserOpenBinding):
		if selected, ok := m.selected(); ok {
			m.detail = true
			m.detailTab = 0
			m.syncDetail(selected)
		}
		return m, nil
	case m.refresh != nil && key.Matches(msg, browserRefreshBinding):
		return m.startRefresh()
	default:
		if handled, cmd := m.runAction(msg.String()); handled {
			return m, cmd
		}
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
}

func (m Browser) View() tea.View {
	content := CenterLayout(m.list.View(), m.width, m.height)
	if m.detail {
		content = m.overlayDetail(content)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m Browser) overlayDetail(background string) string {
	return CenterOverlay(background, m.detailView(), m.width, m.height)
}

func (m Browser) detailView() string {
	selected, ok := m.selected()
	if !ok {
		return Modal(Muted("The selected item is no longer available."), m.modalWidth())
	}
	var builder strings.Builder
	title := strings.TrimSpace(selected.DetailTitle)
	if title == "" {
		title = browserRowTitle(selected)
	}
	builder.WriteString(TwoColumn(Title(title), Secondary(selected.Meta), m.modalContentWidth()))
	if m.err != nil {
		builder.WriteString("\n")
		builder.WriteString(Banner(m.err.Error(), ToneDanger))
	}
	if m.notice != "" {
		builder.WriteString("\n")
		builder.WriteString(Banner(m.notice, ToneSuccess))
	}
	builder.WriteString("\n")
	builder.WriteString(Divider(m.modalContentWidth()))
	if len(selected.DetailTabs) > 1 {
		builder.WriteString("\n")
		builder.WriteString(Tabs(detailTabLabels(selected.DetailTabs), m.detailTab))
		builder.WriteString("\n")
		builder.WriteString(Divider(m.modalContentWidth()))
	}
	builder.WriteString("\n")
	builder.WriteString(m.viewport.View())
	builder.WriteString("\n\n")
	help := []key.Binding{Binding([]string{"j", "k"}, "j/k", "scroll"), Binding([]string{"esc", "q"}, "esc/q", "close")}
	if len(selected.DetailTabs) > 1 {
		help = append([]key.Binding{Binding([]string{"h", "l", "left", "right"}, "←/→", "tabs")}, help...)
	}
	for _, action := range m.actions {
		help = append(help, Binding([]string{action.Key}, action.Key, action.Desc))
	}
	if m.refresh != nil {
		help = append(help, browserRefreshBinding)
	}
	builder.WriteString(DefaultHelp(m.modalContentWidth(), help...))
	return Modal(builder.String(), m.modalWidth())
}

func (m Browser) selected() (Row, bool) {
	item, ok := m.list.SelectedItem().(browserItem)
	if !ok {
		return Row{}, false
	}
	return item.Row, true
}

func (m Browser) rowByID(id string) (Row, bool) {
	for _, item := range m.list.Items() {
		value, ok := item.(browserItem)
		if ok && value.ID == id {
			return value.Row, true
		}
	}
	return Row{}, false
}

func (m Browser) startRefresh() (tea.Model, tea.Cmd) {
	if m.refresh == nil || m.loading {
		return m, nil
	}
	m.loading = true
	m.list.StartSpinner()
	return m, func() tea.Msg {
		rows, err := m.refresh(m.ctx)
		return browserRefreshMsg{rows: rows, err: err}
	}
}

func (m *Browser) runAction(keyValue string) (bool, tea.Cmd) {
	for _, action := range m.actions {
		if action.Key != keyValue {
			continue
		}
		selected, ok := m.selected()
		if !ok {
			return true, nil
		}
		m.notice, m.err = "", nil
		notice, cmd, err := action.Run(selected)
		if err != nil {
			m.err = err
			statusCmd := m.list.NewStatusMessage(err.Error())
			return true, tea.Batch(cmd, statusCmd)
		}
		m.notice = notice
		statusCmd := m.list.NewStatusMessage(notice)
		return true, tea.Batch(cmd, statusCmd)
	}
	return false, nil
}

func (m *Browser) syncHelp() {
	bindings := []key.Binding{browserOpenBinding}
	for _, action := range m.actions {
		bindings = append(bindings, Binding([]string{action.Key}, action.Key, action.Desc))
	}
	if m.refresh != nil {
		bindings = append(bindings, browserRefreshBinding)
	}
	m.list.AdditionalShortHelpKeys = func() []key.Binding { return append([]key.Binding(nil), bindings...) }
	m.list.AdditionalFullHelpKeys = func() []key.Binding { return append([]key.Binding(nil), bindings...) }
}

func (m *Browser) restoreSelection(id string) {
	if id == "" {
		return
	}
	for index, item := range m.list.VisibleItems() {
		value, ok := item.(browserItem)
		if ok && value.ID == id {
			m.list.Select(index)
			return
		}
	}
	if m.detail {
		m.detail = false
	}
}

func (m *Browser) resizeViewport() {
	m.viewport.SetWidth(max(12, m.modalContentWidth()))
	height := m.height
	if height <= 0 {
		height = 20
	}
	m.viewport.SetHeight(max(4, min(14, height-10)))
}

func (m *Browser) syncDetail(row Row) {
	content := ""
	if len(row.DetailTabs) > 0 {
		m.detailTab = MoveTab(m.detailTab, len(row.DetailTabs), 0)
		content = strings.TrimSpace(row.DetailTabs[m.detailTab].Content)
	}
	if content == "" {
		content = strings.TrimSpace(row.Detail)
	}
	if len(row.DetailRows) > 0 {
		content = renderDetailRows(row.DetailRows)
	}
	if content == "" {
		content = strings.TrimSpace(row.Description)
	}
	if content == "" {
		content = row.Summary
	}
	m.resizeViewport()
	m.viewport.SetContent(content)
	m.viewport.GotoTop()
}

func (m *Browser) moveDetailTab(msg tea.KeyPressMsg) bool {
	selected, ok := m.selected()
	if !ok || len(selected.DetailTabs) < 2 {
		return false
	}
	delta, ok := TabDelta(msg)
	if !ok {
		return false
	}
	m.detailTab = MoveTab(m.detailTab, len(selected.DetailTabs), delta)
	m.syncDetail(selected)
	return true
}

func (m Browser) modalWidth() int {
	width := m.width
	if width <= 0 {
		width = 80
	}
	return max(36, min(78, width-10))
}

func (m Browser) modalContentWidth() int { return max(24, m.modalWidth()-6) }

func renderDetailRows(rows []Row) string {
	var builder strings.Builder
	for index, row := range rows {
		if index > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(currentTheme.item.NormalTitle.Render(browserRowTitle(row)))
		if description := strings.TrimSpace(row.Description); description != "" {
			builder.WriteString("\n")
			builder.WriteString(currentTheme.item.NormalDesc.Render(description))
		}
	}
	return builder.String()
}

func detailTabLabels(tabs []DetailTab) []string {
	labels := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		labels = append(labels, tab.Title)
	}
	return labels
}

func browserListItems(rows []Row) []list.Item {
	items := make([]list.Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, browserItem{Row: row})
	}
	return items
}

func browserRowTitle(row Row) string {
	if strings.TrimSpace(row.Title) != "" {
		return row.Title
	}
	if strings.TrimSpace(row.ID) != "" {
		return row.ID
	}
	return row.Summary
}
