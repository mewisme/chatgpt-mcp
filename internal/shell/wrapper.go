package shell

import (
	"context"
	"path/filepath"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/controlguard"
)

type commandPlan struct {
	Requested         string
	Effective         string
	Security          string
	WrapperCapability string
	WrapperProvider   string
	WrapperVersion    string
	WrapperPath       []string
}

func (m *Manager) prepareCommand(ctx context.Context, tool, command string) (commandPlan, error) {
	requested := strings.TrimSpace(command)
	plan := commandPlan{Requested: requested, Effective: requested, Security: requested}
	if _, approved := controlguard.ApprovalFromContext(ctx); approved || m == nil || m.wrappers == nil || requested == "" {
		return plan, nil
	}
	wrapped, err := m.wrappers.Apply(ctx, tool, requested)
	if err != nil {
		return commandPlan{}, err
	}
	plan.Requested = wrapped.Requested
	plan.Effective = wrapped.Effective
	plan.Security = wrapped.Security
	if wrapped.Wrapper != nil {
		plan.WrapperCapability = string(wrapped.Wrapper.Capability)
		plan.WrapperProvider = "plugin/" + string(wrapped.Wrapper.PluginID)
		plan.WrapperVersion = string(wrapped.Wrapper.Version)
		if strings.TrimSpace(wrapped.Wrapper.Path) != "" {
			plan.WrapperPath = []string{filepath.Dir(wrapped.Wrapper.Path)}
		}
	}
	return plan, nil
}
