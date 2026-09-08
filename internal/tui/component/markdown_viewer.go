package component

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
)

type markdownViewerWheelMsg int

type MarkdownViewer struct {
	viewport    viewport.Model
	source      string
	rendered    string
	renderErr   error
	width       int
	height      int
	isDark      bool
	renderCount int
}

var markdownRender = func(source, style string, width int) (string, error) {
	renderer, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(max(1, width)))
	if err != nil {
		return "", err
	}
	defer renderer.Close()
	return renderer.Render(source)
}

func NewMarkdownViewer(source string) MarkdownViewer {
	view := viewport.New(viewport.WithWidth(defaultLayoutWidth), viewport.WithHeight(defaultLayoutHeight))
	view.SoftWrap = false
	view.FillHeight = false
	viewer := MarkdownViewer{viewport: view, source: source, width: defaultLayoutWidth, height: defaultLayoutHeight, isDark: currentTheme.isDark}
	viewer.render(true)
	return viewer
}

func (viewer MarkdownViewer) Init() tea.Cmd { return nil }

func (viewer MarkdownViewer) Update(message tea.Msg) (MarkdownViewer, tea.Cmd) {
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		if viewer.isDark != msg.IsDark() {
			viewer.isDark = msg.IsDark()
			viewer.render(false)
		}
		return viewer, nil
	case tea.WindowSizeMsg:
		viewer.Resize(msg.Width, msg.Height)
		return viewer, nil
	case markdownViewerWheelMsg:
		if msg < 0 {
			viewer.viewport.ScrollUp(3)
		} else if msg > 0 {
			viewer.viewport.ScrollDown(3)
		}
		return viewer, nil
	}
	updated, cmd := viewer.viewport.Update(message)
	viewer.viewport = updated
	return viewer, cmd
}

func (viewer *MarkdownViewer) Resize(width, height int) {
	if viewer == nil {
		return
	}
	width, height = max(1, width), max(1, height)
	widthChanged := viewer.width != width
	viewer.width, viewer.height = width, height
	viewer.viewport.SetWidth(width)
	viewer.viewport.SetHeight(height)
	if widthChanged {
		viewer.render(false)
	}
}

func (viewer *MarkdownViewer) SetSource(source string) {
	if viewer == nil || viewer.source == source {
		return
	}
	viewer.source = source
	viewer.render(true)
}

func (viewer MarkdownViewer) Source() string     { return viewer.source }
func (viewer MarkdownViewer) RenderError() error { return viewer.renderErr }
func (viewer MarkdownViewer) View() string       { return viewer.viewport.View() }

func (viewer MarkdownViewer) MouseTargets(originX, originY, z int) []MouseTarget {
	return []MouseTarget{{
		ID: "markdown.scroll", Rect: Rect{X: originX, Y: originY, Width: max(1, viewer.width), Height: max(1, viewer.height)}, Z: z,
		Handle: func(event MouseEvent) tea.Msg {
			switch event.Button {
			case tea.MouseWheelUp:
				return markdownViewerWheelMsg(-1)
			case tea.MouseWheelDown:
				return markdownViewerWheelMsg(1)
			default:
				return nil
			}
		},
	}}
}

func (viewer *MarkdownViewer) render(reset bool) {
	if viewer == nil {
		return
	}
	offset := viewer.viewport.YOffset()
	style := "dark"
	if !viewer.isDark {
		style = "light"
	}
	value, err := markdownRender(viewer.source, style, max(1, viewer.width))
	viewer.renderCount++
	viewer.renderErr = err
	if err != nil {
		value = WrapContent(viewer.source, max(1, viewer.width))
	}
	viewer.rendered = strings.TrimSpace(value)
	viewer.viewport.SetContent(viewer.rendered)
	if reset {
		viewer.viewport.GotoTop()
		return
	}
	maxOffset := max(0, viewer.viewport.TotalLineCount()-viewer.viewport.Height())
	viewer.viewport.SetYOffset(min(offset, maxOffset))
}
