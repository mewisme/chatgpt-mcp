package page

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type Model interface {
	Init() tea.Cmd
	Update(tea.Msg) (Model, tea.Cmd)
	View(width, height int) string
	OverlayActive() bool
	InputActive() bool
}

type NoticeModel interface {
	Model
	Notice() string
	SetNotice(string)
}

type ToastNoticeModel interface {
	NoticeModel
	ShouldToastNotice() bool
}

type NavigationGuardModel interface {
	Model
	Dirty() bool
	Submitting() bool
}

type SessionViewStateModel interface {
	Model
	SessionViewState() any
	RestoreSessionViewState(any)
}

type NavigateMsg struct {
	Path         []string
	Replace      bool
	PreservePage bool
}

type ToastMsg struct {
	Title   string
	Message string
	Tone    component.Tone
}

type OperationPhase string

const (
	OperationPending   OperationPhase = "pending"
	OperationSuccess   OperationPhase = "success"
	OperationError     OperationPhase = "error"
	OperationCancelled OperationPhase = "cancelled"
)

type OperationMsg struct {
	Key     string
	Phase   OperationPhase
	Title   string
	Message string
	Tone    component.Tone
}

func OperationStarted(key, title, message string) tea.Cmd {
	return func() tea.Msg {
		return OperationMsg{Key: key, Phase: OperationPending, Title: title, Message: message, Tone: component.ToneAccent}
	}
}

func OperationResult(key, title, message string, err error) tea.Msg {
	if err != nil {
		return OperationMsg{Key: key, Phase: OperationError, Title: title, Message: strings.TrimSpace(err.Error()), Tone: component.ToneDanger}
	}
	return OperationMsg{Key: key, Phase: OperationSuccess, Title: title, Message: message, Tone: component.ToneSuccess}
}

func beginOperation(key, title, pending string, work tea.Cmd) tea.Cmd {
	return tea.Batch(OperationStarted(key, title, pending), work)
}

func pageFeedbackHeight(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	return lipgloss.Height(value)
}

func prependPageFeedback(feedback, content string) string {
	if strings.TrimSpace(feedback) == "" {
		return content
	}
	return feedback + "\n" + content
}
