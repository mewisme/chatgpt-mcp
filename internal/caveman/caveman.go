package caveman

import (
	"errors"
	"strings"
	"sync"
)

const Instructions = "CAVEMAN MODE ACTIVE. Use extremely terse, direct language. Prefer short fragments over polished prose. No pleasantries, emojis, or tables. Preserve exact commands, code, paths, identifiers, and error text. Give only the minimum needed to complete the request."

type State struct {
	Active           bool
	InstructionsSent bool
}

type Result struct {
	Available          bool   `json:"available"`
	Active             bool   `json:"active"`
	ActiveInstructions string `json:"active_instructions,omitempty"`
	RefreshHint        string `json:"refresh_hint,omitempty"`
}

type Manager struct {
	mu     sync.Mutex
	states map[string]State
}

func NewManager() *Manager {
	return &Manager{states: map[string]State{}}
}

func (m *Manager) Turn(workspaceID, prompt, action string) (Result, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return Result{}, errors.New("workspace id is required")
	}
	if strings.TrimSpace(prompt) == "" {
		return Result{}, errors.New("prompt is required")
	}
	if action != "turn" && action != "refresh" && action != "status" {
		return Result{}, errors.New("action must be turn, refresh, or status")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.states[workspaceID]
	if active, requested := RequestedState(prompt); requested {
		state.Active = active
		state.InstructionsSent = false
	}
	result := Result{Available: true, Active: state.Active}
	if state.Active {
		if action == "refresh" || !state.InstructionsSent {
			result.ActiveInstructions = Instructions
			state.InstructionsSent = true
		} else {
			result.RefreshHint = "Use action refresh if earlier Caveman instructions are no longer in context."
		}
	}
	m.states[workspaceID] = state
	return result, nil
}

func RequestedState(prompt string) (bool, bool) {
	value := strings.TrimSpace(strings.ToLower(prompt))
	if strings.Contains(value, "stop caveman") || strings.Contains(value, "normal mode") {
		return false, true
	}
	for _, prefix := range []string{"/caveman", "@caveman", "$caveman"} {
		if strings.HasPrefix(value, prefix) {
			rest := strings.TrimSpace(strings.TrimPrefix(value, prefix))
			return rest != "off" && rest != "disable" && rest != "disabled", true
		}
	}
	if value == "caveman" || value == "caveman mode" || strings.HasPrefix(value, "caveman mode on") {
		return true, true
	}
	if strings.HasPrefix(value, "caveman mode off") {
		return false, true
	}
	return false, false
}
