package interactive

import (
	"context"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type Row struct {
	ID      string
	Summary string
	Detail  string
	Search  string
}

type RefreshFunc func(context.Context) ([]Row, error)

type Browser struct {
	ctx       context.Context
	title     string
	rows      []Row
	refresh   RefreshFunc
	keys      ListKeys
	cursor    Cursor
	filter    string
	filtering bool
	detail    bool
	loading   bool
	width     int
	height    int
	err       error
}

type browserRefreshMsg struct {
	rows []Row
	err  error
}

func NewBrowser(ctx context.Context, title string, rows []Row, refresh RefreshFunc) Browser {
	if ctx == nil {
		ctx = context.Background()
	}
	return Browser{ctx: ctx, title: strings.TrimSpace(title), rows: append([]Row(nil), rows...), refresh: refresh, keys: DefaultListKeys()}
}

func (m Browser) Init() tea.Cmd { return nil }

func (m Browser) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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
		if _, ok := m.selected(); ok {
			m.detail = true
		}
	case key.Matches(msg, m.keys.Filter):
		m.filtering = true
	case key.Matches(msg, m.keys.Refresh):
		return m.startRefresh()
	}
	return m, nil
}

func (m Browser) View() tea.View {
	var builder strings.Builder
	builder.WriteString(m.title)
	if m.loading {
		builder.WriteString("  refreshing...")
	}
	builder.WriteString("\n")
	if m.filtering || m.filter != "" {
		builder.WriteString("Filter: ")
		builder.WriteString(m.filter)
		if m.filtering {
			builder.WriteString("_")
		}
		builder.WriteString("\n")
	}
	if m.err != nil {
		builder.WriteString("Error: ")
		builder.WriteString(m.err.Error())
		builder.WriteString("\n")
	}
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
		builder.WriteString("\nNo items match the current filter.\n")
	} else {
		maxRows := m.height - 6
		if maxRows < 3 {
			maxRows = 8
		}
		start, end := visibleWindow(m.cursor.Index, len(items), maxRows)
		for index := start; index < end; index++ {
			prefix := "  "
			if index == m.cursor.Index {
				prefix = "> "
			}
			builder.WriteString(clip(prefix+items[index].Summary, m.width))
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nup/k down/j move  enter/v details  / filter")
	if m.refresh != nil {
		builder.WriteString("  r refresh")
	}
	builder.WriteString("  q quit\n")
}

func (m Browser) writeDetail(builder *strings.Builder) {
	selected, ok := m.selected()
	if !ok {
		m.detail = false
		return
	}
	builder.WriteString("\n")
	builder.WriteString(selected.Summary)
	builder.WriteString("\n\n")
	if strings.TrimSpace(selected.Detail) != "" {
		builder.WriteString(selected.Detail)
		if !strings.HasSuffix(selected.Detail, "\n") {
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nesc/enter back")
	if m.refresh != nil {
		builder.WriteString("  r refresh")
	}
	builder.WriteString("  q back  ctrl+c quit\n")
}

func (m Browser) filtered() []Row {
	query := strings.ToLower(strings.TrimSpace(m.filter))
	if query == "" {
		return m.rows
	}
	result := make([]Row, 0, len(m.rows))
	for _, row := range m.rows {
		haystack := strings.ToLower(strings.Join([]string{row.ID, row.Summary, row.Search}, " "))
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

func clip(value string, width int) string {
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
