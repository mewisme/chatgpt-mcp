package telemetry

import (
	"errors"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/activity"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func AttachTools(runtime *tools.Runtime, stream *activity.Stream, log *logger.Logger) {
	if runtime == nil {
		return
	}
	runtime.SetCallObserver(func(observation tools.CallObservation) {
		fields := []logger.Field{logger.With("tool", observation.Tool)}
		if observation.Source != "" {
			fields = append(fields, logger.With("source", observation.Source))
		}
		if observation.WorkspaceID != "" {
			fields = append(fields, logger.With("workspace", observation.WorkspaceID))
		}
		if observation.SessionHash != "" {
			fields = append(fields, logger.WithVerbose("session", observation.SessionHash))
		}
		if observation.SessionBinding != "" {
			fields = append(fields, logger.WithVerbose("session_binding", string(observation.SessionBinding)))
		}
		if observation.SessionWorkspaceID != "" {
			fields = append(fields, logger.WithVerbose("session_workspace", observation.SessionWorkspaceID))
		}
		if observation.ReceivedByInstanceID != "" {
			fields = append(fields, logger.WithVerbose("received_by", observation.ReceivedByInstanceID))
		}
		if observation.ExecutedByInstanceID != "" {
			fields = append(fields, logger.WithVerbose("executed_by", observation.ExecutedByInstanceID))
		}
		if observation.Phase == "start" {
			if log != nil {
				debugFields := make([]logger.Field, 0, len(fields))
				for _, field := range fields {
					debugFields = append(debugFields, logger.WithDebug(field.Key, field.Value))
				}
				log.Diagnostic(logger.Debug, "TOOL", "tool.call.started", "Tool call started", debugFields...)
			}
			return
		}
		fields = append(fields, logger.With("duration_ms", observation.DurationMS), logger.WithDebug("status", observation.Status))
		if observation.ResultType != "" {
			fields = append(fields, logger.With("result_type", observation.ResultType))
		}
		if log != nil {
			event := logger.Event{Level: logger.Info, Name: "tool.call.completed", Message: "Tool call completed", Fields: fields, Component: "TOOL", Visibility: logger.VisibilityVerbose}
			switch observation.Status {
			case "cancelled":
				event.Level = logger.Warn
				event.Kind = logger.KindWarning
				event.Name = "tool.call.cancelled"
				event.Message = "Tool call cancelled"
			case "error":
				event.Level = logger.Error
				event.Kind = logger.KindError
				event.Name = "tool.call.failed"
				event.Message = "Tool call failed"
			default:
				event.Kind = logger.KindSuccess
			}
			if strings.TrimSpace(observation.Message) != "" && observation.Status != "ok" {
				event.Err = errors.New(strings.TrimSpace(observation.Message))
			}
			log.Emit(event)
		}
		if stream != nil {
			stream.Publish(activity.Event{Kind: string(activity.EventToolCall), Method: "tools/call", Source: observation.Source, Tool: observation.Tool, WorkspaceID: observation.WorkspaceID, SessionHash: observation.SessionHash, SessionBinding: string(observation.SessionBinding), SessionWorkspaceID: observation.SessionWorkspaceID, ReceivedByInstanceID: observation.ReceivedByInstanceID, ExecutedByInstanceID: observation.ExecutedByInstanceID, Status: observation.Status, DurationMS: observation.DurationMS, Message: strings.TrimSpace(observation.Message), Raw: observation.Raw})
		}
	})
}
