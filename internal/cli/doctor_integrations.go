package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/install"
	"go.mewis.me/chatgpt-mcp/internal/redact"
	updatepkg "go.mewis.me/chatgpt-mcp/internal/update"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/version"
	cftunnelplugin "go.mewis.me/chatgpt-mcp/plugins/cf-tunnel"
)

func (d *doctorState) upstreamChecks() []doctorCheck {
	manager := upstream.NewManager(upstream.NewStore(upstream.Path()))
	d.upstreams = manager
	_ = manager.Load()
	checks := []doctorCheck{
		{ID: "upstream.store", Label: "upstream store", Section: "Integrations", Requires: []string{"config.source"}, Run: d.checkUpstreamStore},
	}
	for _, server := range manager.List() {
		id := server.ID
		label := server.Name
		if label == "" {
			label = id
		}
		checks = append(checks, doctorCheck{
			ID: "upstream." + id + ".health", Label: "upstream " + label, Section: "Integrations",
			Requires: []string{"upstream.store"}, Timeout: 3 * time.Second,
			Run: func(ctx context.Context) doctorResult { return d.checkUpstreamHealth(ctx, id) },
		})
	}
	return checks
}

func (d *doctorState) checkUpstreamStore(ctx context.Context) doctorResult {
	if d.upstreams == nil {
		return doctorResult{Status: doctorFail, Summary: "upstream manager is unavailable"}
	}
	if err := d.upstreams.Load(); err != nil {
		return doctorResult{Status: doctorFail, Summary: "upstream store could not be read", Error: redact.Text(err.Error())}
	}
	servers := d.upstreams.List()
	if len(servers) == 0 {
		return doctorResult{Status: doctorSkip, Summary: "no upstream MCP servers configured"}
	}
	return doctorResult{Status: doctorPass, Summary: fmt.Sprintf("%d upstream MCP server(s) configured", len(servers))}
}

func (d *doctorState) checkUpstreamHealth(ctx context.Context, id string) doctorResult {
	if d.upstreams == nil {
		return doctorResult{Status: doctorSkip, Summary: "upstream manager is unavailable"}
	}
	server, ok := d.upstreams.Get(id)
	if !ok {
		return doctorResult{Status: doctorFail, Summary: "upstream server is missing", Details: []string{id}}
	}
	if !server.Enabled {
		return doctorResult{Status: doctorSkip, Summary: "upstream server is disabled", Details: []string{id}}
	}
	status := d.upstreams.CheckHealth(ctx, id, true)
	details := []string{id, string(status.Health)}
	if status.LastError != "" {
		details = append(details, redact.Text(status.LastError))
	}
	switch status.Health {
	case upstream.HealthConnected:
		return doctorResult{Status: doctorPass, Summary: fmt.Sprintf("upstream %s is connected (%d tools)", id, status.ToolCount), Details: details}
	case upstream.HealthDisabled:
		return doctorResult{Status: doctorSkip, Summary: "upstream server is disabled", Details: details}
	default:
		return doctorResult{Status: doctorFail, Summary: "upstream server is unreachable", Error: redact.Text(status.LastError), Details: details}
	}
}

func (d *doctorState) checkTunnelCollection(ctx context.Context) doctorResult {
	collection := d.cfg.RuntimeTunnels()
	if len(collection.Instances) == 0 && len(collection.Admins) == 0 {
		return doctorResult{Status: doctorSkip, Summary: "no Secure MCP tunnels configured"}
	}
	if err := collection.Validate(); err != nil {
		return doctorResult{Status: doctorFail, Summary: "Secure MCP tunnel collection is invalid", Error: redact.Text(err.Error())}
	}
	return doctorResult{Status: doctorPass, Summary: fmt.Sprintf("Secure MCP tunnel collection is valid (%d tunnels, %d admin profiles)", len(collection.Instances), len(collection.Admins))}
}

func (d *doctorState) checkCFTunnelMCP(ctx context.Context) doctorResult {
	return checkCFTunnelTarget(application.CFTunnelSnapshot(d.cfg), cftunnelplugin.TargetMCP)
}

func (d *doctorState) checkCFTunnelAdmin(ctx context.Context) doctorResult {
	return checkCFTunnelTarget(application.CFTunnelSnapshot(d.cfg), cftunnelplugin.TargetAdmin)
}

func checkCFTunnelTarget(snap cftunnelplugin.Snapshot, target string) doctorResult {
	desired := snap.DesiredMCP
	endpoint := snap.MCP
	label := "MCP"
	if target == cftunnelplugin.TargetAdmin {
		desired = snap.DesiredAdmin
		endpoint = snap.Admin
		label = "Admin"
	}
	if !snap.PluginEnabled && !desired {
		return doctorResult{Status: doctorSkip, Summary: "CF Tunnel " + label + " is not enabled"}
	}
	if !desired {
		return doctorResult{Status: doctorSkip, Summary: "CF Tunnel " + label + " target is not desired"}
	}
	if endpoint.AuthErr != nil {
		return doctorResult{Status: doctorFail, Summary: "CF Tunnel " + label + " authentication prerequisite is missing", Error: redact.Text(endpoint.AuthErr.Error()), Hint: "configure Direct MCP HTTP and Admin authentication before exposing a public URL"}
	}
	if !endpoint.Ready {
		return doctorResult{Status: doctorFail, Summary: "CF Tunnel " + label + " listener is not ready", Error: cftunnelplugin.ErrListenerNotReady.Error()}
	}
	return doctorResult{Status: doctorPass, Summary: "CF Tunnel " + label + " prerequisites are satisfied"}
}

func (d *doctorState) checkUpdate(ctx context.Context) doctorResult {
	details := []string{}
	if detection, err := install.DetectCurrent(version.Version); err == nil && detection.Method == install.MethodDirect && detection.Metadata != nil {
		if layout, layoutErr := detection.ManagedLayout(); layoutErr == nil {
			cache, cacheErr := updatepkg.ReadCache(layout.UpdateCache)
			switch {
			case errors.Is(cacheErr, updatepkg.ErrCacheNotFound), cacheErr == nil && cache.Latest == "":
			case cacheErr != nil:
				details = append(details, "cache "+redact.Text(cacheErr.Error()))
			default:
				details = append(details, "cached latest "+cache.Latest)
			}
		}
	}
	result, err := (updatepkg.Checker{}).Check(ctx, version.Version)
	if err != nil {
		return doctorResult{Status: doctorWarn, Summary: "update metadata is unavailable", Error: redact.Text(err.Error()), Details: details}
	}
	summary := "current build is " + string(result.Status)
	if result.Latest != "" {
		details = append(details, "latest "+result.Latest)
	}
	return doctorResult{Status: doctorPass, Summary: summary, Details: details}
}
