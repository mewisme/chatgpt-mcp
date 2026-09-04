package interactive

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
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
	Search      string
}

type RefreshFunc func(context.Context) ([]Row, error)

type RowAction struct {
	Key  string
	Desc string
	Run  func(Row) (string, tea.Cmd, error)
}

type Browser struct {
	ctx       context.Context
	title     string
	rows      []Row
	refresh   RefreshFunc
	actions   []RowAction
	keys      ListKeys
	cursor    Cursor
	viewport  viewport.Model
	filter    string
	filtering bool
	detail    bool
	loading   bool
	width     int
	height    int
	err       error
	notice    string
}

type browserRefreshMsg struct {
	rows []Row
	err  error
}

func NewBrowser(ctx context.Context, title string, rows []Row, refresh RefreshFunc) Browser {
	if ctx == nil {
		ctx = context.Background()
	}
	view := viewport.New(viewport.WithWidth(74), viewport.WithHeight(12))
	view.SoftWrap = true
	view.FillHeight = false
	return Browser{ctx: ctx, title: strings.TrimSpace(title), rows: append([]Row(nil), rows...), refresh: refresh, keys: DefaultListKeys(), viewport: view}
}

func (m Browser) WithAction(action RowAction) Browser {
	action.Key, action.Desc = strings.TrimSpace(action.Key), strings.TrimSpace(action.Desc)
	if action.Key != "" && action.Run != nil {
		m.actions = append(m.actions, action)
	}
	return m
}

func (m Browser) Init() tea.Cmd { return nil }

func (m Browser) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		SetDarkBackground(msg.IsDark())
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeViewport()
	case browserRefreshMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		selectedID := ""
		if selected, ok := m.selected(); ok {
			selectedID = selected.ID
		}
		m.rows = msg.rows
		m.restoreSelection(selectedID)
		if m.detail {
			if selected, ok := m.selected(); ok {
				m.syncViewport(selected)
			}
		}
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Browser) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		keyValue := tea.Key(msg)
		switch msg.String() {
		case "enter", "esc":
			m.filtering = false
		case "backspace":
			m.filter = trimLastRune(m.filter)
			m.cursor.Clamp(len(m.filtered()))
		case "ctrl+u":
			m.filter = ""
			m.cursor.Index = 0
		case "ctrl+c":
			return m, tea.Quit
		default:
			if keyValue.Text != "" {
				m.filter += keyValue.Text
				m.cursor.Index = 0
			}
		}
		return m, nil
	}
	if m.detail {
		switch {
		case msg.String() == "ctrl+c":
			return m, tea.Quit
		case msg.String() == "q", msg.String() == "esc", key.Matches(msg, m.keys.Open):
			m.detail = false
		case key.Matches(msg, m.keys.Refresh):
			return m.startRefresh()
		default:
			if handled, cmd := m.runAction(msg.String()); handled {
				return m, cmd
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	items := m.filtered()
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case msg.String() == "esc":
		if m.filter != "" {
			m.filter = ""
			m.cursor.Index = 0
			return m, nil
		}
		return m, tea.Quit
	case key.Matches(msg, m.keys.Up):
		m.cursor.Move(-1, len(items))
	case key.Matches(msg, m.keys.Down):
		m.cursor.Move(1, len(items))
	case key.Matches(msg, m.keys.Open):
		if selected, ok := m.selected(); ok {
			m.detail = true
			m.syncViewport(selected)
			m.viewport.GotoTop()
		}
	case key.Matches(msg, m.keys.Filter):
		m.filtering = true
	case key.Matches(msg, m.keys.Refresh):
		return m.startRefresh()
	default:
		if handled, cmd := m.runAction(msg.String()); handled {
			return m, cmd
		}
	}
	return m, nil
}

func (m Browser) View() tea.View {
	var builder strings.Builder
	items := m.filtered()
	meta := fmt.Sprintf("%d items", len(items))
	if m.loading {
		meta = Accent("⠋") + " " + Muted("refreshing")
	}
	builder.WriteString(Header(m.title, meta, m.contentWidth()))
	if filter := Filter(m.filter, m.filtering); filter != "" {
		builder.WriteString("\n")
		builder.WriteString(filter)
	}
	if m.err != nil {
		builder.WriteString("\n")
		builder.WriteString(Banner(m.err.Error(), ToneDanger))
	}
	if m.notice != "" {
		builder.WriteString("\n")
		builder.WriteString(Banner(m.notice, ToneSuccess))
	}
	builder.WriteString("\n\n")
	if m.detail {
		m.writeDetail(&builder)
	} else {
		m.writeList(&builder)
	}
	view := tea.NewView(builder.String())
	view.AltScreen = true
	return view
}

func (m Browser) writeList(builder *strings.Builder) {
	items := m.filtered()
	if len(items) == 0 {
		builder.WriteString(Muted("No items match the current filter."))
		builder.WriteString("\n")
	} else {
		maxRows := m.listCapacity()
		start, end := visibleWindow(m.cursor.Index, len(items), maxRows)
		for index := start; index < end; index++ {
			m.writeRow(builder, items[index], index == m.cursor.Index)
		}
	}
	builder.WriteString("\n")
	help := []HelpItem{{Key: "↑/k", Desc: "up"}, {Key: "↓/j", Desc: "down"}, {Key: "enter", Desc: "details"}, {Key: "/", Desc: "filter"}}
	help = append(help, m.actionHelp()...)
	if m.refresh != nil {
		help = append(help, HelpItem{Key: "r", Desc: "refresh"})
	}
	help = append(help, HelpItem{Key: "q", Desc: "quit"})
	builder.WriteString(Help(m.contentWidth(), help...))
	builder.WriteString("\n")
}

func (m Browser) writeRow(builder *strings.Builder, row Row, selected bool) {
	title := row.Title
	if strings.TrimSpace(title) == "" {
		title = row.ID
	}
	if strings.TrimSpace(title) == "" {
		title = row.Summary
	}
	meta := strings.TrimSpace(row.Meta)
	leftWidth := m.contentWidth() - 2
	if meta != "" {
		leftWidth -= utf8.RuneCountInString(meta) + 3
	}
	title = truncatePlain(title, max(8, leftWidth))
	primary := CursorMark(selected) + Primary(title, selected)
	if meta != "" {
		primary = TwoColumn(primary, Secondary(meta), m.contentWidth())
	}
	builder.WriteString(primary)
	builder.WriteString("\n")
	secondary := strings.TrimSpace(row.Description)
	if secondary == "" && row.Title == "" && row.Summary != title {
		secondary = row.Summary
	}
	if secondary != "" {
		builder.WriteString("  ")
		builder.WriteString(SelectedSecondary(truncatePlain(secondary, max(8, m.contentWidth()-2)), selected))
	}
	builder.WriteString("\n")
}

func (m Browser) writeDetail(builder *strings.Builder) {
	selected, ok := m.selected()
	if !ok {
		builder.WriteString(Muted("The selected item is no longer available."))
		return
	}
	builder.WriteString(TwoColumn(Title(browserRowTitle(selected)), Secondary(selected.Meta), m.contentWidth()))
	builder.WriteString("\n\n")
	builder.WriteString(Panel(m.viewport.View(), max(12, m.contentWidth()-2)))
	builder.WriteString("\n\n")
	help := []HelpItem{{Key: "j/k", Desc: "scroll"}, {Key: "pgup/pgdn", Desc: "page"}, {Key: "esc", Desc: "back"}}
	help = append(help, m.actionHelp()...)
	if m.refresh != nil {
		help = append(help, HelpItem{Key: "r", Desc: "refresh"})
	}
	help = append(help, HelpItem{Key: "ctrl+c", Desc: "quit"})
	builder.WriteString(Help(m.contentWidth(), help...))
	builder.WriteString("\n")
}

func (m Browser) filtered() []Row {
	query := strings.ToLower(strings.TrimSpace(m.filter))
	if query == "" {
		return m.rows
	}
	result := make([]Row, 0, len(m.rows))
	for _, row := range m.rows {
		haystack := strings.ToLower(strings.Join([]string{row.ID, row.Title, row.Description, row.Meta, row.Summary, row.Search}, " "))
		if strings.Contains(haystack, query) {
			result = append(result, row)
		}
	}
	return result
}

func (m Browser) selected() (Row, bool) {
	items := m.filtered()
	if len(items) == 0 || m.cursor.Index < 0 || m.cursor.Index >= len(items) {
		return Row{}, false
	}
	return items[m.cursor.Index], true
}

func (m Browser) startRefresh() (tea.Model, tea.Cmd) {
	if m.refresh == nil || m.loading {
		return m, nil
	}
	m.loading = true
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
		m.notice = ""
		m.err = nil
		notice, cmd, err := action.Run(selected)
		if err != nil {
			m.err = err
		} else {
			m.notice = notice
		}
		return true, cmd
	}
	return false, nil
}

func (m Browser) actionHelp() []HelpItem {
	items := make([]HelpItem, 0, len(m.actions))
	for _, action := range m.actions {
		items = append(items, HelpItem{Key: action.Key, Desc: action.Desc})
	}
	return items
}

func (m *Browser) restoreSelection(id string) {
	items := m.filtered()
	for index, row := range items {
		if row.ID == id {
			m.cursor.Index = index
			return
		}
	}
	m.cursor.Clamp(len(items))
	if m.detail && id != "" {
		m.detail = false
	}
}

func (m *Browser) resizeViewport() {
	m.viewport.SetWidth(max(12, m.contentWidth()-6))
	m.viewport.SetHeight(max(4, m.height-10))
}

func (m *Browser) syncViewport(row Row) {
	content := strings.TrimSpace(row.Detail)
	if content == "" {
		content = strings.TrimSpace(row.Description)
	}
	if content == "" {
		content = row.Summary
	}
	m.viewport.SetContent(content)
}

func (m Browser) contentWidth() int {
	if m.width <= 0 {
		return 80
	}
	return max(20, m.width-2)
}

func (m Browser) listCapacity() int {
	if m.height <= 0 {
		return 8
	}
	return max(3, (m.height-9)/2)
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

func visibleWindow(cursor, length, maxRows int) (int, int) {
	if length <= maxRows {
		return 0, length
	}
	start := cursor - maxRows/2
	if start < 0 {
		start = 0
	}
	if start+maxRows > length {
		start = length - maxRows
	}
	return start, start + maxRows
}

func truncatePlain(value string, width int) string {
	if width <= 0 || utf8.RuneCountInString(value) <= width {
		return value
	}
	if width <= 3 {
		return string([]rune(value)[:width])
	}
	return string([]rune(value)[:width-3]) + "..."
}

func trimLastRune(value string) string {
	if value == "" {
		return ""
	}
	_, size := utf8.DecodeLastRuneInString(value)
	return value[:len(value)-size]
}
