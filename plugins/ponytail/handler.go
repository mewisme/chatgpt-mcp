package ponytail

import (
	"context"
	"encoding/json"
	"fmt"

	coreponytail "go.mewis.me/chatgpt-mcp/internal/ponytail"
	"go.mewis.me/chatgpt-mcp/internal/toolprovider"
)

type Handler struct {
	manager *coreponytail.Manager
}

func NewHandler() *Handler {
	return &Handler{manager: coreponytail.NewManager(true, coreponytail.Full)}
}

func (h *Handler) Describe(context.Context, json.RawMessage) (any, error) {
	return toolprovider.DescribeResult{Tools: []toolprovider.Tool{{
		Name: "ponytail_turn", Title: "Ponytail Turn Controller",
		Description:  "Ponytail controller. Call before each user-facing coding response; configured active/mode values seed each workspace state. Pass the exact current user prompt.",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"workspace_id":{"type":"string"},"prompt":{"type":"string"},"action":{"type":"string","enum":["turn","refresh","status"],"default":"turn"}},"required":["workspace_id","prompt"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"available":{"type":"boolean"},"mode":{"type":"string","enum":["off","lite","full","ultra","review"]},"active":{"type":"boolean"},"active_instructions":{"type":"string"},"refresh_hint":{"type":"string"}},"required":["available","mode","active"],"additionalProperties":false}`),
		Risk:         toolprovider.RiskRead,
	}}}, nil
}

func (h *Handler) Status(context.Context, json.RawMessage) (any, error) {
	return map[string]bool{"ok": true}, nil
}
func (h *Handler) Start(context.Context, json.RawMessage) (any, error) {
	return map[string]bool{"ok": true}, nil
}
func (h *Handler) Stop(context.Context, json.RawMessage) (any, error) {
	return map[string]bool{"ok": true}, nil
}
func (h *Handler) Shutdown(context.Context, json.RawMessage) (any, error) {
	return map[string]bool{"ok": true}, nil
}

func (h *Handler) Reconcile(_ context.Context, params json.RawMessage) (any, error) {
	var reconcile toolprovider.ReconcileParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &reconcile); err != nil {
			return nil, err
		}
	}
	h.manager.SetDefaults(boolSetting(reconcile.Settings, "default_active", true), coreponytail.Mode(stringSetting(reconcile.Settings, "default_mode", "full")))
	return map[string]bool{"ok": true}, nil
}

func (h *Handler) Invoke(_ context.Context, params json.RawMessage) (any, error) {
	var invoke toolprovider.InvokeParams
	if err := json.Unmarshal(params, &invoke); err != nil {
		return nil, err
	}
	if invoke.Tool != "ponytail_turn" {
		return nil, fmt.Errorf("unknown tool %q", invoke.Tool)
	}
	workspaceID, _ := invoke.Args["workspace_id"].(string)
	prompt, _ := invoke.Args["prompt"].(string)
	action, _ := invoke.Args["action"].(string)
	if action == "" {
		action = "turn"
	}
	return h.manager.Turn(workspaceID, prompt, action)
}

func boolSetting(values map[string]any, key string, fallback bool) bool {
	raw, ok := values[key]
	if !ok {
		return fallback
	}
	value, ok := raw.(bool)
	if !ok {
		return fallback
	}
	return value
}

func stringSetting(values map[string]any, key, fallback string) string {
	raw, ok := values[key]
	if !ok {
		return fallback
	}
	value, ok := raw.(string)
	if !ok || value == "" {
		return fallback
	}
	return value
}
