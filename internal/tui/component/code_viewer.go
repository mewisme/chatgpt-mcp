package component

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type codeViewerWheelMsg int

type CodeViewer struct {
	viewport viewport.Model
	content  string
	width    int
	height   int
}

func NewCodeViewer(content string) CodeViewer {
	view := viewport.New(viewport.WithWidth(defaultLayoutWidth), viewport.WithHeight(defaultLayoutHeight))
	view.SoftWrap = false
	view.FillHeight = false
	viewer := CodeViewer{viewport: view, content: content, width: defaultLayoutWidth, height: defaultLayoutHeight}
	viewer.viewport.SetContent(content)
	return viewer
}

func (viewer CodeViewer) Init() tea.Cmd { return nil }

func (viewer CodeViewer) Update(message tea.Msg) (CodeViewer, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		viewer.Resize(msg.Width, msg.Height)
		return viewer, nil
	case codeViewerWheelMsg:
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

func (viewer *CodeViewer) Resize(width, height int) {
	if viewer == nil {
		return
	}
	viewer.width, viewer.height = max(1, width), max(1, height)
	viewer.viewport.SetWidth(viewer.width)
	viewer.viewport.SetHeight(viewer.height)
}

func (viewer *CodeViewer) SetContent(content string) {
	if viewer == nil || viewer.content == content {
		return
	}
	viewer.content = content
	viewer.viewport.SetContent(content)
	viewer.viewport.GotoTop()
	viewer.viewport.SetXOffset(0)
}

func (viewer CodeViewer) Content() string { return viewer.content }
func (viewer CodeViewer) View() string    { return viewer.viewport.View() }
func (viewer CodeViewer) XOffset() int    { return viewer.viewport.XOffset() }
func (viewer CodeViewer) YOffset() int    { return viewer.viewport.YOffset() }

func (viewer CodeViewer) MouseTargets(originX, originY, z int) []MouseTarget {
	return []MouseTarget{{
		ID: "code.scroll", Rect: Rect{X: originX, Y: originY, Width: max(1, viewer.width), Height: max(1, viewer.height)}, Z: z,
		Handle: func(event MouseEvent) tea.Msg {
			switch event.Button {
			case tea.MouseWheelUp:
				return codeViewerWheelMsg(-1)
			case tea.MouseWheelDown:
				return codeViewerWheelMsg(1)
			default:
				return nil
			}
		},
	}}
}
