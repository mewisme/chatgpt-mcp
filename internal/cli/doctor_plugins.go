package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/plugindev"
	"go.mewis.me/chatgpt-mcp/internal/redact"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func (d *doctorState) checkPluginLocalDev(_ context.Context) doctorResult {
	ctx, err := plugindev.Detect()
	if err != nil {
		return doctorResult{Status: doctorWarn, Summary: "development plugin context is invalid", Error: redact.Text(err.Error())}
	}
	if !ctx.Enabled {
		return doctorResult{Status: doctorSkip, Summary: "repository local-dev bootstrap is inactive"}
	}
	if d.store == nil {
		return doctorResult{Status: doctorSkip, Summary: "plugin store unavailable"}
	}
	if !strings.Contains(d.store.Layout().ConfigRoot, filepath.Join(".cgm", "dev")) {
		return doctorResult{Status: doctorSkip, Summary: "active plugin store is not the isolated local-dev store"}
	}
	details := []string{"store " + d.store.Layout().ConfigRoot, "doctor does not rebuild local-dev plugins"}
	local := 0
	for _, id := range sortedPluginIDs(d.lock) {
		entry := d.lock.Plugins[id]
		if entry.Registry != pluginpkg.RegistryLocalDev {
			continue
		}
		local++
		installed, err := d.store.Installed(id, entry.Version)
		if err != nil {
			details = append(details, string(id)+" missing")
			continue
		}
		provenance, err := plugindev.ReadProvenance(installed)
		if err != nil {
			details = append(details, string(id)+" local-dev without provenance")
			continue
		}
		details = append(details, string(id)+" local-dev "+provenance.SourceFingerprint)
	}
	if local == 0 {
		return doctorResult{Status: doctorWarn, Summary: "development mode is active but no local-dev plugins are installed", Details: details, Hint: "run a plugin-using command such as cgm plugin list, or set CHATGPT_MCP_DEV_PLUGINS=rebuild"}
	}
	return doctorResult{Status: doctorPass, Summary: fmt.Sprintf("local-dev plugins inspected (%d)", local), Details: details, Hint: "set CHATGPT_MCP_DEV_PLUGINS=rebuild to rebuild; doctor does not repair"}
}

func (d *doctorState) checkPluginPayloads(ctx context.Context) doctorResult {
	if d.store == nil {
		return doctorResult{Status: doctorSkip, Summary: "plugin store unavailable"}
	}
	details := []string{}
	missing, projectionIssues := 0, 0
	ids := sortedPluginIDs(d.lock)
	for _, id := range ids {
		entry := d.lock.Plugins[id]
		installed, err := d.store.Installed(id, entry.Version)
		if err != nil {
			if entry.Enabled {
				missing++
				details = append(details, string(id)+" "+redact.Text(err.Error()))
			}
			continue
		}
		if strings.TrimSpace(installed.Payload) == "" {
			continue
		}
		resources, err := pluginpkg.ProjectionStatus(d.store.Layout(), installed.Payload, entry.Enabled)
		if err != nil {
			projectionIssues++
			details = append(details, string(id)+" "+redact.Text(err.Error()))
			continue
		}
		for _, resource := range resources {
			if resource.State == pluginpkg.ResourceMissing || resource.State == pluginpkg.ResourceConflict || resource.State == pluginpkg.ResourceModified {
				projectionIssues++
				details = append(details, fmt.Sprintf("%s %s/%s %s", id, resource.Kind, resource.Name, resource.State))
			}
		}
	}
	if missing > 0 {
		return doctorResult{Status: doctorFail, Summary: "enabled plugin payloads are missing or unverifiable", Details: details, Hint: "run cgm plugin verify <id> then reinstall or repair"}
	}
	if projectionIssues > 0 {
		return doctorResult{Status: doctorWarn, Summary: "plugin rule/skill projections are unhealthy", Details: details, Hint: "run cgm plugin verify <id>"}
	}
	return doctorResult{Status: doctorPass, Summary: fmt.Sprintf("installed plugin payloads verified (%d)", len(ids)), Details: details}
}

func (d *doctorState) checkPluginCompatibility(ctx context.Context) doctorResult {
	if d.store == nil {
		return doctorResult{Status: doctorSkip, Summary: "plugin store unavailable"}
	}
	manager := &pluginpkg.Manager{Store: d.store}
	issues, err := manager.AssessCoreCompatibility(version.Version)
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "plugin compatibility could not be assessed", Error: redact.Text(err.Error())}
	}
	if len(issues) == 0 {
		return doctorResult{Status: doctorPass, Summary: "enabled plugins are compatible with this core"}
	}
	details := make([]string, 0, len(issues))
	for _, issue := range issues {
		details = append(details, string(issue.ID)+" "+redact.Text(issue.Error))
	}
	return doctorResult{Status: doctorFail, Summary: "enabled plugins are incompatible with this core", Details: details}
}

func (d *doctorState) checkPluginHost(ctx context.Context) doctorResult {
	if d.store == nil {
		return doctorResult{Status: doctorSkip, Summary: "plugin store unavailable"}
	}
	details := []string{}
	failed := 0
	for _, id := range sortedPluginIDs(d.lock) {
		entry := d.lock.Plugins[id]
		if !entry.Enabled {
			continue
		}
		installed, err := d.store.Installed(id, entry.Version)
		if err != nil {
			continue
		}
		artifact, err := installed.Manifest.Platform(runtime.GOOS, runtime.GOARCH)
		if err != nil || !artifact.HostBacked() {
			continue
		}
		if err := pluginpkg.CheckHostPrerequisite(ctx, artifact); err != nil {
			failed++
			details = append(details, string(id)+" "+redact.Text(err.Error()))
		}
	}
	if failed > 0 {
		return doctorResult{Status: doctorFail, Summary: "host-backed plugin executables are missing", Details: details}
	}
	return doctorResult{Status: doctorPass, Summary: "host-backed plugin prerequisites are satisfied"}
}

func (d *doctorState) checkPluginAdminUI(ctx context.Context) doctorResult {
	if !d.cfg.Admin.Enabled {
		return doctorResult{Status: doctorSkip, Summary: "Admin HTTP is disabled"}
	}
	if d.store == nil {
		return doctorResult{Status: doctorSkip, Summary: "plugin store unavailable"}
	}
	id := pluginpkg.PluginID("admin-ui")
	if entry, ok := d.lock.Plugins[id]; ok && entry.Enabled {
		if _, err := d.store.Installed(id, entry.Version); err != nil {
			return doctorResult{Status: doctorFail, Summary: "Admin UI plugin is enabled but its payload is missing", Error: redact.Text(err.Error()), Hint: "run cgm plugin install admin-ui"}
		}
		return doctorResult{Status: doctorPass, Summary: "Admin UI plugin is installed"}
	}
	return doctorResult{Status: doctorWarn, Summary: "Admin HTTP is enabled but admin-ui is not an active plugin", Hint: "run cgm plugin install admin-ui"}
}

func (d *doctorState) checkPluginSecureMCP(_ context.Context) doctorResult {
	collection := d.cfg.RuntimeTunnels()
	if d.store == nil {
		return doctorResult{Status: doctorSkip, Summary: "plugin store unavailable"}
	}
	id := pluginpkg.PluginID("secure-mcp-tunnel")
	if entry, ok := d.lock.Plugins[id]; ok && entry.Enabled {
		if _, err := d.store.Installed(id, entry.Version); err != nil {
			return doctorResult{Status: doctorFail, Summary: "Secure MCP Tunnel plugin is enabled but its payload is missing", Error: redact.Text(err.Error()), Hint: "run cgm plugin install secure-mcp-tunnel"}
		}
		return doctorResult{Status: doctorPass, Summary: "Secure MCP Tunnel plugin is installed"}
	}
	if len(collection.Instances) == 0 && len(collection.Admins) == 0 {
		return doctorResult{Status: doctorSkip, Summary: "Secure MCP Tunnel is not configured"}
	}
	return doctorResult{Status: doctorWarn, Summary: "Secure MCP tunnels are configured but the core plugin is not installed", Hint: "run cgm plugin install secure-mcp-tunnel"}
}

func (d *doctorState) checkPluginTUI(_ context.Context) doctorResult {
	if d.store == nil {
		return doctorResult{Status: doctorSkip, Summary: "plugin store unavailable"}
	}
	id := pluginpkg.PluginID("tui")
	if entry, ok := d.lock.Plugins[id]; ok && entry.Enabled {
		if _, err := d.store.Installed(id, entry.Version); err != nil {
			return doctorResult{Status: doctorFail, Summary: "TUI plugin is enabled but its payload is missing", Error: redact.Text(err.Error()), Hint: "run cgm plugin install tui"}
		}
		return doctorResult{Status: doctorPass, Summary: "TUI plugin is installed"}
	}
	if entry, ok := d.lock.Plugins[id]; ok && !entry.Enabled {
		return doctorResult{Status: doctorWarn, Summary: "TUI core plugin is installed but disabled", Hint: "run cgm plugin enable tui"}
	}
	return doctorResult{Status: doctorWarn, Summary: "TUI core plugin is not installed", Hint: "run cgm plugin install tui"}
}

func (d *doctorState) checkPluginMarkdownFormatter(_ context.Context) doctorResult {
	return d.checkOptionalCorePlugin("markdown-formatter", "Markdown formatter")
}

func (d *doctorState) checkPluginPonytail(_ context.Context) doctorResult {
	return d.checkOptionalCorePlugin("ponytail", "Ponytail")
}

func (d *doctorState) checkPluginCaveman(_ context.Context) doctorResult {
	return d.checkOptionalCorePlugin("caveman", "Caveman")
}

func (d *doctorState) checkOptionalCorePlugin(id, label string) doctorResult {
	if d.store == nil {
		return doctorResult{Status: doctorSkip, Summary: "plugin store unavailable"}
	}
	pluginID := pluginpkg.PluginID(id)
	if entry, ok := d.lock.Plugins[pluginID]; ok && entry.Enabled {
		if _, err := d.store.Installed(pluginID, entry.Version); err != nil {
			return doctorResult{Status: doctorFail, Summary: label + " plugin is enabled but its payload is missing", Error: redact.Text(err.Error()), Hint: "run cgm plugin install " + id}
		}
		return doctorResult{Status: doctorPass, Summary: label + " plugin is installed"}
	}
	if entry, ok := d.lock.Plugins[pluginID]; ok && !entry.Enabled {
		return doctorResult{Status: doctorWarn, Summary: label + " core plugin is installed but disabled", Hint: "run cgm plugin enable " + id}
	}
	return doctorResult{Status: doctorWarn, Summary: label + " core plugin is not installed", Hint: "run cgm plugin install " + id}
}

func sortedPluginIDs(lock pluginpkg.LockFile) []pluginpkg.PluginID {
	ids := make([]pluginpkg.PluginID, 0, len(lock.Plugins))
	for id := range lock.Plugins {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
