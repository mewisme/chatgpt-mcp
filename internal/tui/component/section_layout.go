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
	prefixHeight := lipgloss.Height(prefix)
	return SectionLayout{Header: prefix, BodyY: prefixHeight, BodyHeight: max(1, height-prefixHeight-footerHeight)}
}

func (layout SectionLayout) View(body string) string { return layout.Header + "\n" + body }
