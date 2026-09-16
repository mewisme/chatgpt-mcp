package tools

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func hookCallProvenance(ctx context.Context, callID, source string) (context.Context, pluginpkg.HookProvenance) {
	provenance := pluginpkg.HookProvenance{ExecutionID: strings.TrimSpace(callID), Origin: hookOriginForSource(source)}
	if parent, ok := pluginpkg.HookProvenanceFromContext(ctx); ok {
		provenance.ParentExecutionID = strings.TrimSpace(parent.ExecutionID)
		provenance.Origin = parent.Origin
		provenance.HookDepth = parent.HookDepth
	}
	ctx = pluginpkg.WithHookProvenance(ctx, provenance)
	return ctx, provenance
}

func hookOriginForSource(source string) pluginpkg.HookOrigin {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "http", "sse", "stdio", "tunnel", "mcp":
		return pluginpkg.HookOriginAgent
	default:
		return pluginpkg.HookOriginSystem
	}
}

func (r *Runtime) runPreToolHook(ctx context.Context, provenance pluginpkg.HookProvenance, name, workspaceID string, args map[string]any) (pluginpkg.HookResult, error) {
	if r == nil || r.Hooks == nil || name == ApprovalRequestToolName {
		return pluginpkg.HookResult{Schema: pluginpkg.HookSchema, Decision: pluginpkg.HookDecisionContinue}, nil
	}
	result, err := r.Hooks.PreToolUse(ctx, r.hookEvent(pluginpkg.HookEventPreToolUse, provenance, name, workspaceID, args, nil, nil, 0, ""))
	if err != nil {
		return result, controlguard.New(controlguard.CodeHookPolicy, "pre-tool hook failed: "+err.Error(), false, nil)
	}
	switch result.Decision {
	case pluginpkg.HookDecisionContinue:
		return result, nil
	case pluginpkg.HookDecisionDeny:
		return result, controlguard.New(controlguard.CodeHookPolicy, "pre-tool hook denied execution: "+strings.TrimSpace(result.Reason), false, nil)
	case pluginpkg.HookDecisionRequireApproval:
		if grant, ok := controlguard.GrantFromContext(ctx); ok && grant.Code == controlguard.CodeHookPolicy {
			return result, nil
		}
		return result, controlguard.New(controlguard.CodeHookPolicy, "pre-tool hook requires approval: "+strings.TrimSpace(result.Reason), true, nil)
	default:
		return result, controlguard.New(controlguard.CodeHookPolicy, "pre-tool hook returned an unsupported decision", false, nil)
	}
}

func (r *Runtime) observeToolHook(eventType pluginpkg.HookEventType, provenance pluginpkg.HookProvenance, name, workspaceID string, args map[string]any, result *Result, err error, started time.Time, status string) {
	if r == nil || r.Hooks == nil || name == ApprovalRequestToolName {
		return
	}
	r.Hooks.Observe(r.hookEvent(eventType, provenance, name, workspaceID, args, result, err, time.Since(started), status))
}

func (r *Runtime) hookEvent(eventType pluginpkg.HookEventType, provenance pluginpkg.HookProvenance, name, workspaceID string, args map[string]any, result *Result, err error, duration time.Duration, status string) pluginpkg.HookEvent {
	event := pluginpkg.HookEvent{
		Type: eventType, Provenance: provenance, Tool: name, WorkspaceID: workspaceID, CWD: r.hookCWD(name, workspaceID),
		RequestedArguments: cloneMap(args), EffectiveArguments: cloneMap(args), SecurityCommand: hookSecurityCommand(args), DurationMS: duration.Milliseconds(), Status: status,
	}
	if result != nil {
		event.Result = &pluginpkg.HookResultMetadata{IsError: result.IsError, ResultType: result.ResultType, ContentCount: len(result.Content)}
	}
	if err != nil {
		event.Error = err.Error()
	} else if result != nil && result.IsError && len(result.Content) > 0 {
		event.Error = result.Content[0].Text
	}
	return event
}

func (r *Runtime) hookCWD(name, workspaceID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	if r == nil || workspaceID == "" {
		return ""
	}
	if (name == "run_command" || name == "start_process") && r.Shell != nil {
		if status, err := r.Shell.Status(workspaceID); err == nil {
			return status.CWD
		}
	}
	if r.Workspaces != nil {
		if item, err := r.Workspaces.Get(workspaceID); err == nil {
			return item.Path
		}
	}
	return ""
}

func hookSecurityCommand(args map[string]any) string {
	command, _ := args["command"].(string)
	return strings.TrimSpace(command)
}

func hookObservationType(registryCalled bool, result Result, err error) pluginpkg.HookEventType {
	if !registryCalled {
		return pluginpkg.HookEventToolDenied
	}
	if err != nil {
		if _, denied := controlguard.As(err); denied {
			return pluginpkg.HookEventToolDenied
		}
		return pluginpkg.HookEventToolError
	}
	if result.IsError {
		return pluginpkg.HookEventToolError
	}
	return pluginpkg.HookEventPostToolUse
}

func hookObservationStatus(eventType pluginpkg.HookEventType) string {
	switch eventType {
	case pluginpkg.HookEventPostToolUse:
		return "ok"
	case pluginpkg.HookEventToolDenied:
		return "denied"
	case pluginpkg.HookEventToolError:
		return "error"
	default:
		return ""
	}
}

func hookObservationError(eventType pluginpkg.HookEventType, result Result, err error) error {
	if err != nil {
		return err
	}
	if eventType == pluginpkg.HookEventToolDenied || eventType == pluginpkg.HookEventToolError {
		if len(result.Content) > 0 && strings.TrimSpace(result.Content[0].Text) != "" {
			return errors.New(result.Content[0].Text)
		}
		return errors.New(string(eventType))
	}
	return nil
}
