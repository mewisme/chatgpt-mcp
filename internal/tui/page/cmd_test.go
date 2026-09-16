package page

import tea "charm.land/bubbletea/v2"

func navigateMsg(cmd tea.Cmd) (NavigateMsg, bool) {
	if cmd == nil {
		return NavigateMsg{}, false
	}
	return findNavigate(cmd())
}

func findNavigate(msg tea.Msg) (NavigateMsg, bool) {
	if nav, ok := msg.(NavigateMsg); ok {
		return nav, true
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return NavigateMsg{}, false
	}
	for _, item := range batch {
		if item == nil {
			continue
		}
		if nav, ok := findNavigate(item()); ok {
			return nav, true
		}
	}
	return NavigateMsg{}, false
}
