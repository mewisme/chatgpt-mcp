package component

import "charm.land/lipgloss/v2"

type SectionLayout struct {
	Header     string
	BodyY      int
	BodyHeight int
}

func NewSectionLayout(title, meta, feedback string, width, height, footerHeight int) SectionLayout {
	width, height, footerHeight = max(1, width), max(1, height), max(0, footerHeight)
	header := TwoColumn(Title(title), Secondary(meta), width)
	prefix := header
	if feedback != "" {
		prefix += "\n" + feedback
	}
	prefix += "\n" + Divider(width)
	prefixHeight := lipgloss.Height(header) + 1 + lipgloss.Height(Divider(width))
	if feedback != "" {
		prefixHeight += lipgloss.Height(feedback) + 1
	}
	return SectionLayout{Header: prefix, BodyY: lipgloss.Height(prefix) + 1, BodyHeight: max(1, height-prefixHeight-footerHeight-1)}
}

func (layout SectionLayout) View(body string) string { return layout.Header + "\n" + body }
