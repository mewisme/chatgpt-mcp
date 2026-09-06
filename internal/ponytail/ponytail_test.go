package ponytail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/hooks"
)

func TestRequestedMode(t *testing.T) {
	cases := map[string]Mode{
		"/ponytail lite":             Lite,
		"@ponytail full":             Full,
		"$ponytail ultra":            Ultra,
		"/ponytail-review":           Review,
		"/ponytail :ponytail-review": Review,
		"/ponytail off":              Off,
		"stop ponytail":              Off,
		"normal mode":                Off,
	}
	for prompt, expected := range cases {
		value, ok := RequestedMode(prompt)
		if !ok || value != expected {
			t.Fatalf("%q => %q %v, want %q", prompt, value, ok, expected)
		}
	}
	if _, ok := RequestedMode("normal user prompt"); ok {
		t.Fatal("normal prompt should not request a mode")
	}
}

func TestModeFromInstructions(t *testing.T) {
	for input, expected := range map[string]Mode{
		"PONYTAIL MODE ACTIVE — level: lite":   Lite,
		"PONYTAIL MODE ACTIVE - level: full":   Full,
		"PONYTAIL MODE ACTIVE - level: ultra":  Ultra,
		"PONYTAIL MODE ACTIVE — level: review": Review,
		"PONYTAIL MODE ACTIVE - level: off":    Off,
	} {
		if value := ModeFromInstructions(input); value != expected {
			t.Fatalf("%q => %q, want %q", input, value, expected)
		}
	}
	if value := ModeFromInstructions("no marker"); value != Full {
		t.Fatalf("default mode = %q", value)
	}
}

func TestPonytailHooksSelectsFirstTrustedEnabledHooks(t *testing.T) {
	values := []hooks.Hook{
		{ID: "ignored-plugin", Plugin: "other@plugin", Event: hooks.SessionStart, Trusted: true, Enabled: true},
		{ID: "ignored-untrusted", Plugin: "ponytail@ponytail", Event: hooks.SessionStart, Enabled: true},
		{ID: "activation", Plugin: "ponytail@ponytail", Event: hooks.SessionStart, Trusted: true, Enabled: true},
		{ID: "activation-second", Plugin: "ponytail@ponytail", Event: hooks.SessionStart, Trusted: true, Enabled: true},
		{ID: "tracker", Plugin: "ponytail@ponytail", Event: hooks.UserPromptSubmit, Trusted: true, Enabled: true},
	}
	activation, tracker := ponytailHooks(values)
	if activation == nil || activation.ID != "activation" || tracker == nil || tracker.ID != "tracker" {
		t.Fatalf("activation=%#v tracker=%#v", activation, tracker)
	}
}

func TestInstructionsFor(t *testing.T) {
	if value := instructionsFor(context.Background(), t.TempDir(), Off, t.TempDir()); value != "" {
		t.Fatalf("off instructions = %q", value)
	}
	root := t.TempDir()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	module := `module.exports.getPonytailInstructions = mode => "instructions:" + mode`
	if err := os.WriteFile(filepath.Join(root, "hooks", "ponytail-instructions.js"), []byte(module), 0644); err != nil {
		t.Fatal(err)
	}
	if value := instructionsFor(context.Background(), root, Lite, workspace); value != "instructions:lite" {
		t.Fatalf("instructions = %q", value)
	}
}

func TestManagerTurnLifecycle(t *testing.T) {
	oldDiscover, oldRun := discoverHooks, runHook
	defer func() { discoverHooks, runHook = oldDiscover, oldRun }()
	activation := hooks.Hook{ID: "activation", Plugin: "ponytail@ponytail", Event: hooks.SessionStart, Trusted: true, Enabled: true}
	tracker := hooks.Hook{ID: "tracker", Plugin: "ponytail@ponytail", Event: hooks.UserPromptSubmit, Trusted: true, Enabled: true}
	discoverHooks = func() ([]hooks.Hook, error) { return []hooks.Hook{activation, tracker}, nil }
	runCalls := 0
	runHook = func(_ context.Context, hook hooks.Hook, _ string, input string) string {
		runCalls++
		if hook.ID == tracker.ID {
			if !strings.Contains(input, "/ponytail off") {
				t.Fatalf("tracker input = %q", input)
			}
			return ""
		}
		return "PONYTAIL MODE ACTIVE - level: ultra\nactivation instructions"
	}
	manager := NewManager(true)
	result, err := manager.Turn(context.Background(), "ws", t.TempDir(), "hello", "status")
	if err != nil || !result.Available || result.Mode != Ultra || !result.Active || result.ActiveInstructions == "" {
		t.Fatalf("first result=%#v err=%v", result, err)
	}
	result, err = manager.Turn(context.Background(), "ws", t.TempDir(), "hello again", "status")
	if err != nil || result.ActiveInstructions != "" || result.RefreshHint == "" {
		t.Fatalf("second result=%#v err=%v", result, err)
	}
	result, err = manager.Turn(context.Background(), "ws", t.TempDir(), "/ponytail off", "turn")
	if err != nil || result.Mode != Off || result.Active || result.ActiveInstructions != "" {
		t.Fatalf("off result=%#v err=%v", result, err)
	}
	if runCalls != 2 {
		t.Fatalf("run calls = %d", runCalls)
	}
}

func TestManagerHonorsConfiguredInactiveState(t *testing.T) {
	oldDiscover, oldRun := discoverHooks, runHook
	defer func() { discoverHooks, runHook = oldDiscover, oldRun }()
	activation := hooks.Hook{ID: "activation", Plugin: "ponytail@ponytail", Event: hooks.SessionStart, Trusted: true, Enabled: true}
	discoverHooks = func() ([]hooks.Hook, error) { return []hooks.Hook{activation}, nil }
	runCalls := 0
	runHook = func(context.Context, hooks.Hook, string, string) string {
		runCalls++
		return "PONYTAIL MODE ACTIVE - level: ultra"
	}
	manager := NewManager(false)
	result, err := manager.Turn(context.Background(), "ws", t.TempDir(), "continue", "status")
	if err != nil || !result.Available || result.Active || result.Mode != Off || result.ActiveInstructions != "" {
		t.Fatalf("inactive result=%#v err=%v", result, err)
	}
	if runCalls != 0 {
		t.Fatalf("inactive manager invoked activation hook %d times", runCalls)
	}
}

func TestManagerTurnValidationAndUnavailable(t *testing.T) {
	oldDiscover := discoverHooks
	defer func() { discoverHooks = oldDiscover }()
	manager := NewManager()
	if _, err := manager.Turn(context.Background(), "ws", t.TempDir(), "", "status"); err == nil {
		t.Fatal("empty prompt accepted")
	}
	if _, err := manager.Turn(context.Background(), "ws", t.TempDir(), "x", "bad"); err == nil {
		t.Fatal("invalid action accepted")
	}
	discoverHooks = func() ([]hooks.Hook, error) { return nil, errors.New("discover failed") }
	if _, err := manager.Turn(context.Background(), "ws", t.TempDir(), "x", "status"); err == nil || !strings.Contains(err.Error(), "discover failed") {
		t.Fatalf("discover error = %v", err)
	}
	discoverHooks = func() ([]hooks.Hook, error) { return nil, nil }
	result, err := manager.Turn(context.Background(), "ws", t.TempDir(), "x", "status")
	if err != nil || result.Available || result.Error == "" {
		t.Fatalf("unavailable result=%#v err=%v", result, err)
	}
}

func TestUnavailablePonytailStillReportsConfiguredActiveState(t *testing.T) {
	oldDiscover := discoverHooks
	defer func() { discoverHooks = oldDiscover }()
	discoverHooks = func() ([]hooks.Hook, error) { return nil, nil }
	manager := NewManager(true)
	result, err := manager.Turn(context.Background(), "ws", t.TempDir(), "continue", "status")
	if err != nil || result.Available || !result.Active || result.Mode != Full {
		t.Fatalf("unavailable active result=%#v err=%v", result, err)
	}
}
