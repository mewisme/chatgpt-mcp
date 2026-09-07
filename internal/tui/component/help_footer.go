package component

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

const defaultHelpShortLimit = 5

type HelpFooter struct {
	bindings   []key.Binding
	expanded   bool
	shortLimit int
}

func NewHelpFooter(bindings ...key.Binding) HelpFooter {
	footer := HelpFooter{shortLimit: defaultHelpShortLimit}
	footer.SetBindings(bindings...)
	return footer
}

func (footer *HelpFooter) SetBindings(bindings ...key.Binding) {
	if footer == nil {
		return
	}
	footer.bindings = append(footer.bindings[:0], bindings...)
	if footer.shortLimit <= 0 {
		footer.shortLimit = defaultHelpShortLimit
	}
	if len(footer.enabledBindings()) <= footer.shortLimit {
		footer.expanded = false
	}
}

func (footer HelpFooter) Expanded() bool { return footer.expanded }

func (footer *HelpFooter) SetExpanded(expanded bool) {
	if footer == nil {
		return
	}
	footer.expanded = expanded && len(footer.enabledBindings()) > footer.shortLimit
}

func (footer *HelpFooter) Update(message tea.Msg) bool {
	if footer == nil || len(footer.enabledBindings()) <= footer.shortLimit {
		return false
	}
	msg, ok := message.(tea.KeyPressMsg)
	if !ok || msg.String() != "?" {
		return false
	}
	footer.expanded = !footer.expanded
	return true
}

func (footer HelpFooter) View(width int) string {
	bindings := footer.enabledBindings()
	if len(bindings) == 0 {
		return ""
	}
	model := help.New()
	model.Styles = help.DefaultStyles(currentTheme.isDark)
	model.SetWidth(width)
	if len(bindings) <= footer.shortLimit {
		return model.ShortHelpView(bindings)
	}
	toggle := Binding([]string{"?"}, "?", "more")
	if footer.expanded {
		toggle = Binding([]string{"?"}, "?", "less")
		return model.FullHelpView([][]key.Binding{append(bindings, toggle)})
	}
	visible := max(0, footer.shortLimit-1)
	short := append([]key.Binding(nil), bindings[:min(visible, len(bindings))]...)
	short = append(short, toggle)
	return model.ShortHelpView(short)
}

func (footer HelpFooter) enabledBindings() []key.Binding {
	bindings := make([]key.Binding, 0, len(footer.bindings))
	for _, binding := range footer.bindings {
		help := binding.Help()
		if binding.Enabled() && strings.TrimSpace(help.Key+help.Desc) != "" {
			bindings = append(bindings, binding)
		}
	}
	return bindings
}
