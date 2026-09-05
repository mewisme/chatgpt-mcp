package interactive

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
)

func NewDefaultList(title string, items []list.Item, width, height int, singular, plural string) list.Model {
	delegate := list.NewDefaultDelegate()
	model := list.New(items, delegate, width, height)
	ApplyDefaultListTheme(&model, true)
	model.Title = title
	model.KeyMap.Quit = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit"))
	model.KeyMap.ForceQuit = key.NewBinding(key.WithKeys("ctrl+c"))
	model.SetStatusBarItemName(singular, plural)
	return model
}

func ApplyDefaultTextInputTheme(model *textinput.Model, isDark bool) {
	if model != nil {
		model.SetStyles(textinput.DefaultStyles(isDark))
	}
}

func ApplyDefaultListTheme(model *list.Model, isDark bool) {
	SetDarkBackground(isDark)
	model.Styles = list.DefaultStyles(isDark)
	delegate := list.NewDefaultDelegate()
	delegate.Styles = list.NewDefaultItemStyles(isDark)
	model.SetDelegate(delegate)
}

func Binding(keys []string, helpKey, description string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(helpKey, description))
}

func DefaultHelp(width int, bindings ...key.Binding) string {
	model := help.New()
	model.Styles = help.DefaultStyles(currentTheme.isDark)
	model.SetWidth(width)
	return model.ShortHelpView(bindings)
}
