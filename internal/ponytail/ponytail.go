package ponytail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/hooks"
)

type Mode string

const (
	Off    Mode = "off"
	Lite   Mode = "lite"
	Full   Mode = "full"
	Ultra  Mode = "ultra"
	Review Mode = "review"
)

type State struct {
	Mode             Mode   `json:"mode"`
	Instructions     string `json:"instructions,omitempty"`
	InstructionsSent bool   `json:"-"`
}

type Result struct {
	Available          bool   `json:"available"`
	Mode               Mode   `json:"mode,omitempty"`
	Active             bool   `json:"active,omitempty"`
	ActiveInstructions string `json:"active_instructions,omitempty"`
	RefreshHint        string `json:"refresh_hint,omitempty"`
	Error              string `json:"error,omitempty"`
}

type Manager struct {
	mu     sync.Mutex
	states map[string]State
}

var modePattern = regexp.MustCompile(`(?i)PONYTAIL MODE ACTIVE\s*[—-]\s*level:\s*(lite|full|ultra|review|off)`)

func NewManager() *Manager {
	return &Manager{states: map[string]State{}}
}

func (m *Manager) Turn(ctx context.Context, workspaceID, workspaceRoot, prompt, action string) (Result, error) {
	if strings.TrimSpace(prompt) == "" {
		return Result{}, errors.New("prompt is required")
	}
	if action != "turn" && action != "refresh" && action != "status" {
		return Result{}, errors.New("action must be turn, refresh, or status")
	}
	all, err := hooks.Discover()
	if err != nil {
		return Result{}, err
	}
	activation, tracker := ponytailHooks(all)
	if activation == nil {
		return Result{Available: false, Error: "Ponytail plugin or its trusted SessionStart hook is disabled"}, nil
	}

	requested, hasRequested := RequestedMode(prompt)
	m.mu.Lock()
	state, exists := m.states[workspaceID]
	m.mu.Unlock()

	if hasRequested && tracker != nil {
		payload, _ := json.Marshal(map[string]string{"prompt": prompt})
		_ = hooks.Run(ctx, *tracker, workspaceRoot, string(payload))
	}
	switch {
	case hasRequested && requested == Off:
		state = State{Mode: Off}
		exists = true
	case hasRequested:
		instructions := instructionsFor(ctx, activation.PluginRoot, requested, workspaceRoot)
		state = State{Mode: requested, Instructions: instructions}
		exists = true
	case !exists:
		instructions := hooks.Run(ctx, *activation, workspaceRoot, "")
		state = State{Mode: ModeFromInstructions(instructions), Instructions: instructions}
		exists = true
	}

	includeInstructions := action == "refresh" || !state.InstructionsSent
	state.InstructionsSent = true
	m.mu.Lock()
	m.states[workspaceID] = state
	m.mu.Unlock()

	result := Result{Available: true, Mode: state.Mode, Active: state.Mode != Off}
	if includeInstructions {
		result.ActiveInstructions = state.Instructions
	} else {
		result.RefreshHint = "Use action refresh if earlier Ponytail instructions are no longer in context."
	}
	return result, nil
}

func RequestedMode(prompt string) (Mode, bool) {
	value := strings.TrimSpace(strings.ToLower(prompt))
	if strings.Contains(value, "stop ponytail") || strings.Contains(value, "normal mode") {
		return Off, true
	}
	prefix := ""
	for _, candidate := range []string{"/ponytail", "@ponytail", "$ponytail"} {
		if strings.HasPrefix(value, candidate) {
			prefix = candidate
			break
		}
	}
	if prefix == "" {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	rest = strings.TrimSpace(strings.TrimPrefix(rest, ":ponytail"))
	if strings.HasPrefix(rest, "-review") {
		return Review, true
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", false
	}
	switch fields[0] {
	case "lite":
		return Lite, true
	case "full":
		return Full, true
	case "ultra":
		return Ultra, true
	case "off":
		return Off, true
	default:
		return "", false
	}
}

func ModeFromInstructions(instructions string) Mode {
	match := modePattern.FindStringSubmatch(instructions)
	if len(match) < 2 {
		return Full
	}
	switch strings.ToLower(match[1]) {
	case "lite":
		return Lite
	case "ultra":
		return Ultra
	case "review":
		return Review
	case "off":
		return Off
	default:
		return Full
	}
}

func ponytailHooks(values []hooks.Hook) (*hooks.Hook, *hooks.Hook) {
	var activation, tracker *hooks.Hook
	for index := range values {
		value := &values[index]
		if value.Plugin != "ponytail@ponytail" || !value.Enabled || !value.Trusted {
			continue
		}
		switch value.Event {
		case hooks.SessionStart:
			if activation == nil {
				activation = value
			}
		case hooks.UserPromptSubmit:
			if tracker == nil {
				tracker = value
			}
		}
	}
	return activation, tracker
}

func instructionsFor(ctx context.Context, pluginRoot string, mode Mode, workspaceRoot string) string {
	if mode == Off {
		return ""
	}
	modulePath := filepath.Join(pluginRoot, "hooks", "ponytail-instructions.js")
	if info, err := os.Stat(modulePath); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return ""
	}
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	script := `const m=require(process.argv[1]); const v=m.getPonytailInstructions?.(process.argv[2]); if (typeof v === "string") process.stdout.write(v);`
	cmd := exec.CommandContext(runCtx, node, "-e", script, modulePath, string(mode))
	cmd.Dir = workspaceRoot
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}
