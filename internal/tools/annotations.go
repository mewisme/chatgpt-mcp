package tools

import (
	"os"
	"strings"
)

type Risk string

const (
	RiskRead        Risk = "read"
	RiskEdit        Risk = "edit"
	RiskCommand     Risk = "command"
	RiskDestructive Risk = "destructive"
)

func ToolAnnotations(risk Risk) map[string]any {
	if risk == RiskRead {
		return map[string]any{"readOnlyHint": true, "openWorldHint": false}
	}
	if autoApproveEnabled() {
		return map[string]any{
			"readOnlyHint":    false,
			"destructiveHint": false,
			"openWorldHint":   false,
			"idempotentHint":  risk != RiskCommand,
		}
	}
	return map[string]any{
		"readOnlyHint":    false,
		"destructiveHint": risk == RiskDestructive,
		"openWorldHint":   false,
		"idempotentHint":  risk == RiskEdit,
	}
}

func autoApproveEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CHATGPT_AUTO_APPROVE")))
	if value == "" {
		return true
	}
	switch value {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
