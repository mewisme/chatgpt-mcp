package page

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"go.mewis.me/chatgpt-mcp/docs/tuiguide"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type GuidePage struct {
	ctx     context.Context
	topic   tuiguide.Topic
	browser component.Browser
	viewer  component.MarkdownViewer
	width   int
	height  int
}

func NewGuide(ctx context.Context, topicID string) (*GuidePage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	page := &GuidePage{ctx: ctx}
	if strings.TrimSpace(topicID) == "" {
		rows := make([]component.Row, 0, len(tuiguide.Topics()))
		for _, topic := range tuiguide.Topics() {
			rows = append(rows, component.Row{ID: topic.ID, Title: topic.Title, Description: topic.Description, Search: strings.Join(append([]string{topic.ID, topic.Title}, topic.Keywords...), " ")})
		}
		page.browser = component.NewBrowser(ctx, "TUI Guide", rows, nil).WithTitleVisible(false).WithExternalHelp(true)
		return page, nil
	}
	topic, ok := tuiguide.Lookup(topicID)
	if !ok {
		return nil, &guideTopicError{id: topicID}
	}
	markdown, err := tuiguide.Markdown(topic.ID)
	if err != nil {
		return nil, err
	}
	page.topic = topic
	page.viewer = component.NewMarkdownViewer(markdown)
	return page, nil
}

type guideTopicError struct{ id string }

func (err *guideTopicError) Error() string {
	return "unknown TUI guide topic: " + strings.TrimSpace(err.id)
}

func (page *GuidePage) Init() tea.Cmd {
	if page == nil || page.topic.ID == "" {
		return nil
	}
	return page.viewer.Init()
}

func (page *GuidePage) OverlayActive() bool { return false }
func (page *GuidePage) InputActive() bool {
	return page != nil && page.topic.ID == "" && page.browser.InputActive()
}

func (page *GuidePage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		page.width, page.height = msg.Width, msg.Height
		page.resize()
		return page, nil
	case component.BrowserOpenMsg:
		if page.topic.ID == "" && msg.Row.ID != "" {
			return page, func() tea.Msg { return NavigateMsg{Path: []string{"guide", msg.Row.ID}} }
		}
	case tea.BackgroundColorMsg:
		if page.topic.ID == "" {
			updated, cmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			return page, cmd
		}
		updated, cmd := page.viewer.Update(msg)
		page.viewer = updated
		return page, cmd
	}
	if page.topic.ID == "" {
		updated, cmd := page.browser.Update(message)
		page.browser = updated.(component.Browser)
		return page, cmd
	}
	updated, cmd := page.viewer.Update(message)
	page.viewer = updated
	return page, cmd
}

func (page *GuidePage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "TUI Guide unavailable", "")
	}
	page.width, page.height = width, height
	page.resize()
	if page.topic.ID == "" {
		help := page.browser.HelpView()
		layout := component.NewSectionLayout("TUI Guide", "embedded · "+guideTopicCountLabel(len(tuiguide.Topics())), "", width, height, lipgloss.Height(help))
		body := component.BottomHelp(layout.View(page.browser.BodyContent()), help, width, height)
		return body
	}
	help := component.DefaultHelp(width, component.Binding([]string{"j", "k"}, "j/k", "scroll"), component.Binding([]string{"esc"}, "esc", "topics"))
	title := component.PageTitle("Guide · "+page.topic.Title, width)
	contentHeight := max(1, height-lipgloss.Height(title)-1-lipgloss.Height(help))
	page.viewer.Resize(width, contentHeight)
	content := title + "\n" + page.viewer.View()
	return component.BottomHelp(content, help, width, height)
}

func (page *GuidePage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil || page.width <= 0 || page.height <= 0 {
		return nil
	}
	if page.topic.ID == "" {
		help := page.browser.HelpView()
		layout := component.NewSectionLayout("TUI Guide", "embedded · "+guideTopicCountLabel(len(tuiguide.Topics())), "", page.width, page.height, lipgloss.Height(help))
		targets := page.browser.MouseTargets(originX, originY+layout.BodyY, z)
		helpY := originY + page.height - lipgloss.Height(help)
		return append(targets, page.browser.HelpMouseTargets(originX, helpY, z+2)...)
	}
	help := component.DefaultHelp(page.width, component.Binding([]string{"j", "k"}, "j/k", "scroll"), component.Binding([]string{"esc"}, "esc", "topics"))
	title := component.PageTitle("Guide · "+page.topic.Title, page.width)
	viewerY := originY + lipgloss.Height(title) + 1
	viewerHeight := max(1, page.height-lipgloss.Height(title)-1-lipgloss.Height(help))
	page.viewer.Resize(page.width, viewerHeight)
	return page.viewer.MouseTargets(originX, viewerY, z)
}

func (page *GuidePage) resize() {
	if page == nil || page.width <= 0 || page.height <= 0 {
		return
	}
	if page.topic.ID == "" {
		help := page.browser.HelpView()
		layout := component.NewSectionLayout("TUI Guide", "embedded · "+guideTopicCountLabel(len(tuiguide.Topics())), "", page.width, page.height, lipgloss.Height(help))
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: layout.BodyHeight})
		page.browser = updated.(component.Browser)
		return
	}
	help := component.DefaultHelp(page.width, component.Binding([]string{"j", "k"}, "j/k", "scroll"), component.Binding([]string{"esc"}, "esc", "topics"))
	title := component.PageTitle("Guide · "+page.topic.Title, page.width)
	page.viewer.Resize(page.width, max(1, page.height-lipgloss.Height(title)-1-lipgloss.Height(help)))
}

func guideTopicCountLabel(count int) string {
	if count == 1 {
		return "1 topic"
	}
	return strconv.Itoa(count) + " topics"
}
