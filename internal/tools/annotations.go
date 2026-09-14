package tools

type Risk string

const (
	RiskRead        Risk = "read"
	RiskEdit        Risk = "edit"
	RiskCommand     Risk = "command"
	RiskDestructive Risk = "destructive"
)

func ToolAnnotations(risk Risk) map[string]any {
	return map[string]any{
		"readOnlyHint":    risk == RiskRead,
		"destructiveHint": risk == RiskDestructive || risk == RiskCommand,
		"openWorldHint":   risk == RiskCommand,
		"idempotentHint":  risk == RiskRead,
	}
}

func ToolAnnotationsOpenWorld(risk Risk) map[string]any {
	annotations := ToolAnnotations(risk)
	annotations["openWorldHint"] = true
	return annotations
}
