package component

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	helpBindings       []key.Binding
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

type browserMouseMsg struct {
	Index int
	Tab   int
	Wheel int
	Open  bool
}

var browserOpenBinding = Binding([]string{"enter", "v"}, "enter", "details")
var browserRefreshBinding = Binding([]string{"r"}, "r", "refresh")

func NewBrowser(ctx context.Context, title string, rows []Row, refresh RefreshFunc) Browser {
	if ctx == nil {
		ctx = context.Background()
	}
	model := NewDefaultList(strings.TrimSpace(title), browserListItems(rows), 80, 20, "item", "items")
	model.InfiniteScrolling = true
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

func (m Browser) WithHelpBindings(bindings ...key.Binding) Browser {
	m.SetHelpBindings(bindings...)
	return m
}

func (m *Browser) SetHelpBindings(bindings ...key.Binding) {
	if m == nil {
		return
	}
	m.helpBindings = append([]key.Binding(nil), bindings...)
	m.syncHelp()
}

func (m Browser) WithTitleVisible(visible bool) Browser {
	m.list.SetShowTitle(visible)
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
	case browserMouseMsg:
		return m.handleMouse(msg)
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
		case msg.String() == "esc", key.Matches(msg, browserOpenBinding):
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
	view := tea.NewView(m.Content())
	view.AltScreen = true
	return view
}

func (m Browser) Content() string {
	content := m.list.View()
	if m.detail {
		content = m.overlayDetail(content)
	}
	return content
}

func (m Browser) Selected() (Row, bool) { return m.selected() }

func (m *Browser) ReplaceRows(rows []Row, selectedID string) tea.Cmd {
	if m == nil {
		return nil
	}
	m.list.SetShowStatusBar(len(rows) > 0)
	cmd := m.list.SetItems(browserListItems(rows))
	if cmd != nil {
		m.pendingSelectionID = selectedID
	} else if selectedID != "" {
		m.restoreSelection(selectedID)
	}
	if m.detail {
		if selected, ok := m.rowByID(selectedID); ok {
			m.syncDetail(selected)
		} else {
			m.detail = false
		}
	}
	return cmd
}

func (m *Browser) SelectLast() bool {
	if m == nil {
		return false
	}
	items := m.list.VisibleItems()
	if len(items) == 0 {
		return false
	}
	m.list.Select(len(items) - 1)
	return true
}

func (m *Browser) SelectID(id string) bool {
	if m == nil {
		return false
	}
	for index, item := range m.list.VisibleItems() {
		value, ok := item.(browserItem)
		if ok && value.ID == id {
			m.list.Select(index)
			return true
		}
	}
	return false
}

func (m *Browser) OpenDetail(id string) bool {
	if m == nil || !m.SelectID(id) {
		return false
	}
	selected, ok := m.selected()
	if !ok {
		return false
	}
	m.detail = true
	m.detailTab = 0
	m.syncDetail(selected)
	return true
}

func (m Browser) DetailOpen() bool { return m.detail }

func (m Browser) InputActive() bool { return m.list.FilterState() == list.Filtering }

func (m Browser) MouseTargets(originX, originY, z int) []MouseTarget {
	if m.width <= 0 || m.height <= 0 {
		return nil
	}
	targets := []MouseTarget{{
		ID: "browser.scroll", Rect: Rect{X: originX, Y: originY, Width: m.width, Height: m.height}, Z: z,
		Handle: func(event MouseEvent) tea.Msg {
			switch event.Button {
			case tea.MouseWheelUp:
				return browserMouseMsg{Wheel: -1}
			case tea.MouseWheelDown:
				return browserMouseMsg{Wheel: 1}
			default:
				return nil
			}
		},
	}}
	if m.detail {
		return append(targets, m.detailMouseTargets(originX, originY, z+10)...)
	}
	startY := 0
	if m.list.ShowTitle() || m.list.ShowFilter() {
		startY += 1 + m.list.Styles.TitleBar.GetPaddingTop() + m.list.Styles.TitleBar.GetPaddingBottom()
	}
	if m.list.ShowStatusBar() {
		startY += 1 + m.list.Styles.StatusBar.GetPaddingTop() + m.list.Styles.StatusBar.GetPaddingBottom()
	}
	visible := m.list.VisibleItems()
	start, end := m.list.Paginator.GetSliceBounds(len(visible))
	for index := start; index < end; index++ {
		rowIndex := index
		y := originY + startY + (index-start)*3
		open := index == m.list.GlobalIndex()
		targets = append(targets, MouseTarget{
			ID: "browser.row", Rect: Rect{X: originX, Y: y, Width: m.width, Height: 2}, Z: z + 1,
			Handle: func(event MouseEvent) tea.Msg {
				if event.Button != tea.MouseLeft {
					return nil
				}
				return browserMouseMsg{Index: rowIndex, Open: open}
			},
		})
	}
	viewLines := strings.Split(ansi.Strip(m.list.View()), "\n")
	helpBindings := append([]key.Binding(nil), m.helpBindings...)
	for _, action := range m.actions {
		helpBindings = append(helpBindings, Binding([]string{action.Key}, action.Key, action.Desc))
	}
	if m.refresh != nil {
		helpBindings = append(helpBindings, browserRefreshBinding)
	}
	for _, binding := range helpBindings {
		help := binding.Help()
		label := strings.TrimSpace(help.Key + " " + help.Desc)
		keys := binding.Keys()
		if label == "" || len(keys) == 0 {
			continue
		}
		line, column := findRenderedLine(viewLines, label, 0)
		if line < 0 {
			continue
		}
		keyValue := keys[0]
		targets = append(targets, MouseTarget{
			ID: "browser.help", Rect: Rect{X: originX + column, Y: originY + line, Width: lipgloss.Width(label), Height: 1}, Z: z + 2,
			Handle: func(event MouseEvent) tea.Msg {
				if event.Button != tea.MouseLeft {
					return nil
				}
				return browserHelpKeyMsg(keyValue)
			},
		})
	}
	return targets
}

func browserHelpKeyMsg(value string) tea.KeyPressMsg {
	switch value {
	case "space", " ":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		runes := []rune(value)
		if len(runes) == 0 {
			return tea.KeyPressMsg{}
		}
		return tea.KeyPressMsg{Code: runes[0]}
	}
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
	help := []key.Binding{Binding([]string{"j", "k"}, "j/k", "scroll"), Binding([]string{"esc"}, "esc", "close")}
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

func (m Browser) detailMouseTargets(originX, originY, z int) []MouseTarget {
	detail := m.detailView()
	width, height := lipgloss.Width(detail), lipgloss.Height(detail)
	x := originX + max(0, (m.width-width)/2)
	y := originY + max(0, (m.height-height)/2)
	targets := []MouseTarget{{
		ID: "browser.detail.scroll", Rect: Rect{X: x, Y: y, Width: width, Height: height}, Z: z,
		Handle: func(event MouseEvent) tea.Msg {
			switch event.Button {
			case tea.MouseWheelUp:
				return browserMouseMsg{Wheel: -1}
			case tea.MouseWheelDown:
				return browserMouseMsg{Wheel: 1}
			default:
				return nil
			}
		},
	}}
	selected, ok := m.selected()
	if !ok || len(selected.DetailTabs) < 2 {
		return targets
	}
	lines := strings.Split(ansi.Strip(detail), "\n")
	for tabIndex, tab := range selected.DetailTabs {
		label := strings.TrimSpace(tab.Title)
		if label == "" {
			continue
		}
		line, column := findRenderedLine(lines, label, 0)
		if line < 0 {
			continue
		}
		index := tabIndex
		targets = append(targets, MouseTarget{
			ID: "browser.detail.tab", Rect: Rect{X: x + column, Y: y + line, Width: lipgloss.Width(label), Height: 1}, Z: z + 1,
			Handle: func(event MouseEvent) tea.Msg {
				if event.Button != tea.MouseLeft {
					return nil
				}
				return browserMouseMsg{Tab: index + 1}
			},
		})
	}
	return targets
}

func (m Browser) handleMouse(msg browserMouseMsg) (tea.Model, tea.Cmd) {
	if msg.Tab > 0 && m.detail {
		selected, ok := m.selected()
		if ok && msg.Tab-1 < len(selected.DetailTabs) {
			m.detailTab = msg.Tab - 1
			m.syncDetail(selected)
		}
		return m, nil
	}
	if msg.Wheel != 0 {
		if m.detail {
			if msg.Wheel < 0 {
				m.viewport.ScrollUp(3)
			} else {
				m.viewport.ScrollDown(3)
			}
		} else if msg.Wheel < 0 {
			m.list.CursorUp()
		} else {
			m.list.CursorDown()
		}
		return m, nil
	}
	if msg.Index >= 0 && msg.Index < len(m.list.VisibleItems()) {
		m.list.Select(msg.Index)
		if msg.Open {
			if selected, ok := m.selected(); ok {
				m.detail = true
				m.detailTab = 0
				m.syncDetail(selected)
			}
		}
	}
	return m, nil
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
	bindings = append(bindings, m.helpBindings...)
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
	m.viewport.SetWidth(max(1, m.modalContentWidth()))
	height := m.height
	if height <= 0 {
		height = 20
	}
	m.viewport.SetHeight(max(1, min(14, height-10)))
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
	return max(1, min(78, width-2))
}

func (m Browser) modalContentWidth() int { return max(1, m.modalWidth()-6) }

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
