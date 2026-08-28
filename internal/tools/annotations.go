package tools

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
	return map[string]any{
		"readOnlyHint":    false,
		"destructiveHint": false,
		"openWorldHint":   false,
		"idempotentHint":  risk != RiskCommand,
	}
}
