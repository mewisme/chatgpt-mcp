package tools

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/jsruntime"
	"go.mewis.me/chatgpt-mcp/internal/ponytail"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type NodeResetResult struct {
	Reset bool `json:"reset"`
}

func RegisterAdvancedTools(registry *Registry, workspaces *workspace.Manager) {
	nodeManager := jsruntime.NewManager()
	ponytailManager := ponytail.NewManager()
	register := func(name, title, description, input, output string, risk Risk, handler Handler) {
		registry.MustRegister(name, Schema{
			Name: name, Title: title, Description: description,
			InputSchema: json.RawMessage(input), OutputSchema: json.RawMessage(output), Annotations: ToolAnnotations(risk),
		}, handler)
	}

	register("node_repl", "Node REPL", "Stateful JavaScript session per workspace. globalThis persists across calls. Filesystem access is constrained by the Node permission model to the registered workspace.", `{"type":"object","properties":{"workspace_id":{"type":"string"},"action":{"type":"string","enum":["eval","reset","status"],"default":"eval"},"code":{"type":"string"},"timeout_ms":{"type":"integer","minimum":100,"maximum":60000,"default":30000}},"required":["workspace_id"],"additionalProperties":false}`, `{"type":"object","additionalProperties":true}`, RiskEdit, func(ctx context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		action, err := optionalEnum(args, "action", "eval", "eval", "reset", "status")
		if err != nil {
			return Result{}, err
		}
		switch action {
		case "reset":
			if err := nodeManager.Reset(item.ID); err != nil {
				return Result{}, err
			}
			return JSONResult(NodeResetResult{Reset: true}), nil
		case "status":
			value, err := nodeManager.Status(ctx, item.ID, item.Path)
			if err != nil {
				return Result{}, err
			}
			return JSONResult(value), nil
		default:
			code, err := optionalString(args, "code")
			if err != nil {
				return Result{}, err
			}
			if code == "" {
				return Result{}, errors.New("code is required for node_repl eval")
			}
			timeoutMS, err := optionalInt(args, "timeout_ms", 30000, 100, 60000)
			if err != nil {
				return Result{}, err
			}
			callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS+2000)*time.Millisecond)
			defer cancel()
			value, err := nodeManager.Eval(callCtx, item.ID, item.Path, code, time.Duration(timeoutMS)*time.Millisecond)
			if err != nil {
				return Result{}, err
			}
			return JSONResult(value), nil
		}
	})

	register("ponytail_turn", "Ponytail Turn Controller", "Call before each user-facing response when the trusted Ponytail plugin is enabled. Pass the exact current user prompt.", `{"type":"object","properties":{"workspace_id":{"type":"string"},"prompt":{"type":"string"},"action":{"type":"string","enum":["turn","refresh","status"],"default":"turn"}},"required":["workspace_id","prompt"],"additionalProperties":false}`, `{"type":"object","properties":{"available":{"type":"boolean"},"mode":{"type":"string"},"active":{"type":"boolean"},"active_instructions":{"type":"string"},"refresh_hint":{"type":"string"},"error":{"type":"string"}},"required":["available"],"additionalProperties":false}`, RiskRead, func(ctx context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		prompt, err := requiredString(args, "prompt")
		if err != nil {
			return Result{}, err
		}
		action, err := optionalEnum(args, "action", "turn", "turn", "refresh", "status")
		if err != nil {
			return Result{}, err
		}
		value, err := ponytailManager.Turn(ctx, item.ID, item.Path, prompt, action)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(value), nil
	})
}
