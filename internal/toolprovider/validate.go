package toolprovider

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

func ValidateDescribe(result DescribeResult) error {
	if len(result.Tools) == 0 {
		return fmt.Errorf("tool-provider must describe at least one tool")
	}
	if len(result.Tools) > MaxTools {
		return fmt.Errorf("tool-provider describes %d tools, max %d", len(result.Tools), MaxTools)
	}
	seen := map[string]struct{}{}
	for _, tool := range result.Tools {
		if err := ValidateTool(tool); err != nil {
			return err
		}
		if _, ok := seen[tool.Name]; ok {
			return fmt.Errorf("duplicate tool %q", tool.Name)
		}
		seen[tool.Name] = struct{}{}
	}
	return nil
}

func ValidateTool(tool Tool) error {
	if err := validateName(tool.Name); err != nil {
		return err
	}
	if strings.TrimSpace(tool.Title) == "" {
		return fmt.Errorf("tool %q title is required", tool.Name)
	}
	if err := validateSchema(tool.Name, "input", tool.InputSchema); err != nil {
		return err
	}
	if err := validateSchema(tool.Name, "output", tool.OutputSchema); err != nil {
		return err
	}
	if _, err := NormalizeRisk(tool.Risk); err != nil {
		return fmt.Errorf("tool %q: %w", tool.Name, err)
	}
	return nil
}

func NormalizeRisk(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "", RiskRead:
		return RiskRead, nil
	case RiskEdit, RiskCommand, RiskDestructive:
		return strings.TrimSpace(value), nil
	default:
		return "", fmt.Errorf("unknown risk %q", value)
	}
}

func FloorRisk(declared string, permissions []string) string {
	risk, _ := NormalizeRisk(declared)
	floor := RiskRead
	for _, permission := range permissions {
		switch strings.TrimSpace(permission) {
		case "filesystem/workspace-write":
			floor = maxRisk(floor, RiskEdit)
		case "process/execute":
			floor = maxRisk(floor, RiskCommand)
		}
	}
	return maxRisk(risk, floor)
}

func validateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > MaxNameBytes {
		return fmt.Errorf("invalid tool name %q", name)
	}
	for i, r := range name {
		if i == 0 && (r < 'a' || r > 'z') {
			return fmt.Errorf("invalid tool name %q", name)
		}
		if r != '_' && !unicode.IsLower(r) && (r < '0' || r > '9') {
			return fmt.Errorf("invalid tool name %q", name)
		}
	}
	return nil
}

func validateSchema(tool, kind string, schema json.RawMessage) error {
	if len(schema) == 0 {
		return fmt.Errorf("tool %q %s schema is required", tool, kind)
	}
	if len(schema) > MaxSchemaBytes {
		return fmt.Errorf("tool %q %s schema exceeds %d bytes", tool, kind, MaxSchemaBytes)
	}
	var value map[string]any
	if err := json.Unmarshal(schema, &value); err != nil {
		return fmt.Errorf("tool %q %s schema: %w", tool, kind, err)
	}
	if value["type"] != "object" {
		return fmt.Errorf("tool %q %s schema must be a JSON object type", tool, kind)
	}
	return nil
}

func riskRank(value string) int {
	switch value {
	case RiskEdit:
		return 1
	case RiskCommand:
		return 2
	case RiskDestructive:
		return 3
	default:
		return 0
	}
}

func maxRisk(a, b string) string {
	if riskRank(b) > riskRank(a) {
		return b
	}
	return a
}
