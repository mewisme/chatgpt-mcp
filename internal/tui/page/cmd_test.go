package page

import tea "charm.land/bubbletea/v2"

func navigateMsg(cmd tea.Cmd) (NavigateMsg, bool) {
	if cmd == nil {
		return NavigateMsg{}, false
	}
	return findNavigate(cmd())
}

func operationMsg(cmd tea.Cmd) (OperationMsg, bool) {
	if cmd == nil {
		return OperationMsg{}, false
	}
	return findOperation(cmd())
}

func workMsg(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, item := range batch {
			if item == nil {
				continue
			}
			inner := item()
			if _, ok := inner.(OperationMsg); ok {
				continue
			}
			return inner
		}
		return nil
	}
	return msg
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

func findOperation(msg tea.Msg) (OperationMsg, bool) {
	if op, ok := msg.(OperationMsg); ok {
		return op, true
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return OperationMsg{}, false
	}
	for _, item := range batch {
		if item == nil {
			continue
		}
		if op, ok := findOperation(item()); ok {
			return op, true
		}
	}
	return OperationMsg{}, false
}
