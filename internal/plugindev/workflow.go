package plugindev

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type CorePlugin struct {
	ID        string
	Required  bool
	Enabled   bool
	Platforms []string
}

type workflowFile struct {
	Plugins []workflowPlugin `json:"plugins"`
}

type workflowPlugin struct {
	ID   string        `json:"id"`
	Core *workflowCore `json:"core"`
}

type workflowCore struct {
	Required  bool     `json:"required"`
	Enabled   *bool    `json:"enabled"`
	Platforms []string `json:"platforms"`
}

func LoadCorePlugins(repoRoot string) ([]CorePlugin, error) {
	root, err := verifiedRoot(repoRoot)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(root, "plugins", "workflow.json"))
	if err != nil {
		return nil, fmt.Errorf("dev plugin discover: %w", err)
	}
	var workflow workflowFile
	if err := json.Unmarshal(data, &workflow); err != nil {
		return nil, fmt.Errorf("dev plugin discover: %w", err)
	}
	out := make([]CorePlugin, 0)
	for _, plugin := range workflow.Plugins {
		if plugin.Core == nil || plugin.ID == "" {
			continue
		}
		enabled := true
		if plugin.Core.Enabled != nil {
			enabled = *plugin.Core.Enabled
		}
		out = append(out, CorePlugin{
			ID: plugin.ID, Required: plugin.Core.Required, Enabled: enabled,
			Platforms: append([]string(nil), plugin.Core.Platforms...),
		})
	}
	return out, nil
}

func (plugin CorePlugin) AllowsPlatform(platform string) bool {
	if len(plugin.Platforms) == 0 {
		return true
	}
	for _, allowed := range plugin.Platforms {
		if allowed == platform {
			return true
		}
	}
	return false
}
