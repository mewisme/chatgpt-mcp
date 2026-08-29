package telemetry

import (
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
		fields := []any{"tool", observation.Tool}
		if observation.Source != "" {
			fields = append(fields, "source", observation.Source)
		}
		if observation.WorkspaceID != "" {
			fields = append(fields, "workspace", observation.WorkspaceID)
		}
		if observation.Phase == "start" {
			if log != nil {
				log.Info("TOOL", "call started", fields...)
			}
			return
		}

		fields = append(fields, "duration_ms", observation.DurationMS)
		if observation.ResultType != "" {
			fields = append(fields, "result_type", observation.ResultType)
		}
		if log != nil {
			switch observation.Status {
			case "cancelled":
				log.Warn("TOOL", "call cancelled", append(fields, "error", observation.Message)...)
			case "error":
				log.Error("TOOL", "call failed", append(fields, "error", observation.Message)...)
			default:
				log.Success("TOOL", "call completed", fields...)
			}
		}
		if stream != nil {
			stream.Publish(activity.Event{
				Kind:        string(activity.EventToolCall),
				Method:      "tools/call",
				Source:      observation.Source,
				Tool:        observation.Tool,
				WorkspaceID: observation.WorkspaceID,
				Status:      observation.Status,
				DurationMS:  observation.DurationMS,
				Message:     strings.TrimSpace(observation.Message),
			})
		}
	})
}
