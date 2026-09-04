package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/cli/interactive"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func runInteractiveBrowser(cmd *cobra.Command, title string, rows []interactive.Row, refresh interactive.RefreshFunc, actions ...interactive.RowAction) error {
	model := interactive.NewBrowser(cmd.Context(), title, rows, refresh)
	for _, action := range actions {
		model = model.WithAction(action)
	}
	_, err := interactive.Run(cmd.Context(), model, cmd.InOrStdin(), cmd.OutOrStdout())
	return err
}

func workspaceCopyIDAction() interactive.RowAction {
	return interactive.RowAction{Key: "c", Desc: "copy ID", Run: func(row interactive.Row) (string, tea.Cmd, error) {
		return "Copied " + row.ID, tea.SetClipboard(row.ID), nil
	}}
}

func workspaceInteractiveRows(items []workspace.Workspace) []interactive.Row {
	rows := make([]interactive.Row, 0, len(items))
	for _, item := range items {
		meta := ""
		if len(item.AllowDirs) > 0 {
			meta = fmt.Sprintf("%d extra roots", len(item.AllowDirs))
		}
		rows = append(rows, interactive.Row{ID: item.ID, Title: item.ID, Description: item.Path, Meta: meta, Summary: fmt.Sprintf("%-18s %s", item.ID, item.Path), DetailTitle: "Workspace · " + item.ID, DetailRows: workspaceInteractiveDetailRows(item), Search: strings.Join(append(append([]string{}, item.AllowDirs...), item.LegacyIDs...), " ")})
	}
	return rows
}

func workspaceInteractiveDetailRows(item workspace.Workspace) []interactive.Row {
	rows := []interactive.Row{{ID: "root", Title: "Root", Description: item.Path}}
	if len(item.AllowDirs) == 0 {
		rows = append(rows, interactive.Row{ID: "additional-roots", Title: "Additional roots", Description: "None"})
	} else {
		for index, root := range item.AllowDirs {
			rows = append(rows, interactive.Row{ID: fmt.Sprintf("additional-root-%d", index), Title: "Additional root", Description: root})
		}
	}
	if len(item.LegacyIDs) == 0 {
		rows = append(rows, interactive.Row{ID: "legacy-ids", Title: "Legacy IDs", Description: "None"})
	} else {
		for index, id := range item.LegacyIDs {
			rows = append(rows, interactive.Row{ID: fmt.Sprintf("legacy-id-%d", index), Title: "Legacy ID", Description: id})
		}
	}
	return rows
}

func upstreamInteractiveRows(items []upstream.Server) []interactive.Row {
	rows := make([]interactive.Row, 0, len(items))
	for _, item := range items {
		view := redactUpstreamServer(item)
		endpoint := view.URL
		if view.Transport == "stdio" {
			endpoint = view.Command
		}
		state := "disabled"
		if view.Enabled {
			state = "enabled"
		}
		summary := fmt.Sprintf("%-18s %-6s enabled=%t expose=%s endpoint=%s", view.ID, view.Transport, view.Enabled, view.Expose, endpoint)
		rows = append(rows, interactive.Row{ID: view.ID, Title: view.ID, Description: endpoint, Meta: strings.Join([]string{view.Transport, state, "expose " + view.Expose}, " · "), Summary: summary, DetailTitle: "MCP server · " + view.ID, DetailTabs: upstreamServerDetailTabs(view), Search: strings.Join([]string{view.Name, view.Transport, view.Expose, endpoint}, " ")})
	}
	return rows
}

func upstreamStatusInteractiveRows(items []upstream.Status) []interactive.Row {
	rows := make([]interactive.Row, 0, len(items))
	for _, item := range items {
		description := item.Name
		if item.LastError != "" {
			description = item.LastError
		}
		summary := fmt.Sprintf("%-18s %-6s enabled=%t health=%s tools=%d expose=%s", item.ID, item.Transport, item.Enabled, item.Health, item.ToolCount, item.Expose)
		meta := fmt.Sprintf("%s · %s · %d tools", item.Transport, item.Health, item.ToolCount)
		rows = append(rows, interactive.Row{ID: item.ID, Title: item.ID, Description: description, Meta: meta, Summary: summary, DetailTitle: "MCP status · " + item.ID, DetailTabs: upstreamStatusDetailTabs(item), Search: strings.Join([]string{item.Name, item.Transport, string(item.Health), item.Expose, item.LastError}, " ")})
	}
	return rows
}

func tunnelInteractiveRows(items []tunnel.Metadata) []interactive.Row {
	rows := make([]interactive.Row, 0, len(items))
	for _, item := range items {
		scope := strings.Join(append(append([]string{}, item.OrganizationIDs...), item.WorkspaceIDs...), ",")
		if scope == "" {
			scope = strings.Join(item.TenantIDs, ",")
		}
		title := item.Name
		if strings.TrimSpace(title) == "" {
			title = item.ID
		}
		description := item.ID
		if item.Description != "" {
			description += " · " + item.Description
		}
		summary := fmt.Sprintf("%-22s %-24s scope=%s", item.ID, item.Name, scope)
		rows = append(rows, interactive.Row{ID: item.ID, Title: title, Description: description, Meta: scope, Summary: summary, DetailTitle: "Tunnel · " + item.ID, DetailTabs: tunnelDetailTabs(item), Search: strings.Join([]string{item.Name, item.Description, item.Creator, scope}, " ")})
	}
	return rows
}

type interactiveDetailField struct {
	label string
	value string
}

func upstreamServerDetailTabs(item upstream.Server) []interactive.DetailTab {
	endpoint := item.URL
	if item.Transport == "stdio" {
		endpoint = item.Command
	}
	overview := interactiveDetailFields(
		interactiveDetailField{"ID", item.ID}, interactiveDetailField{"Name", item.Name}, interactiveDetailField{"Transport", item.Transport},
		interactiveDetailField{"Enabled", fmt.Sprintf("%t", item.Enabled)}, interactiveDetailField{"Expose", item.Expose}, interactiveDetailField{"Tool prefix", item.ToolPrefix},
		interactiveDetailField{"Idle timeout", fmt.Sprintf("%ds", item.IdleTimeoutSec)},
	)
	connectionFields := []interactiveDetailField{{"Endpoint", endpoint}, {"Auth", item.Auth.Type}, {"Auth scope", emptyInteractiveValue(item.Auth.Scope)}, {"CWD", emptyInteractiveValue(item.CWD)}, {"Bearer env", emptyInteractiveValue(item.BearerTokenEnvVar)}}
	if len(item.Args) > 0 {
		connectionFields = append(connectionFields, interactiveDetailField{"Args", strings.Join(item.Args, " ")})
	}
	connection := interactiveDetailFields(connectionFields...)
	if len(item.Headers) > 0 {
		connection += "\n\n" + interactive.Title("Headers") + "\n" + interactiveDetailMap(item.Headers)
	}
	if len(item.Env) > 0 {
		connection += "\n\n" + interactive.Title("Environment") + "\n" + interactiveDetailMap(item.Env)
	}
	tools := interactive.Title("Allowlisted tools") + "\n" + interactiveDetailList(item.Tools) + "\n\n" + interactive.Title("Disabled tools") + "\n" + interactiveDetailList(item.DisabledTools)
	return []interactive.DetailTab{{Title: "Overview", Content: overview}, {Title: "Connection", Content: connection}, {Title: "Tools", Content: tools}}
}

func upstreamStatusDetailTabs(item upstream.Status) []interactive.DetailTab {
	pid := "-"
	if item.PID != nil {
		pid = fmt.Sprintf("%d", *item.PID)
	}
	overview := interactiveDetailFields(
		interactiveDetailField{"ID", item.ID}, interactiveDetailField{"Name", item.Name}, interactiveDetailField{"Transport", item.Transport}, interactiveDetailField{"Auth", item.Auth},
		interactiveDetailField{"Enabled", fmt.Sprintf("%t", item.Enabled)}, interactiveDetailField{"Health", string(item.Health)}, interactiveDetailField{"Connected", fmt.Sprintf("%t", item.Connected)},
		interactiveDetailField{"Tool count", fmt.Sprintf("%d", item.ToolCount)}, interactiveDetailField{"Expose", item.Expose}, interactiveDetailField{"PID", pid},
	)
	tools := interactiveDetailList(item.ProxiedTools)
	errorContent := interactive.Muted(emptyInteractiveValue(item.LastError))
	return []interactive.DetailTab{{Title: "Overview", Content: overview}, {Title: "Tools", Content: tools}, {Title: "Error", Content: errorContent}}
}

func tunnelDetailTabs(item tunnel.Metadata) []interactive.DetailTab {
	overview := interactiveDetailFields(
		interactiveDetailField{"ID", item.ID}, interactiveDetailField{"Name", item.Name}, interactiveDetailField{"Description", emptyInteractiveValue(item.Description)},
		interactiveDetailField{"Creator", emptyInteractiveValue(item.Creator)}, interactiveDetailField{"Request", emptyInteractiveValue(item.RequestID)}, interactiveDetailField{"Fetched", interactiveTime(item.FetchedAt)},
	)
	scope := interactive.Title("Organizations") + "\n" + interactiveDetailList(item.OrganizationIDs) + "\n\n" + interactive.Title("Workspaces") + "\n" + interactiveDetailList(item.WorkspaceIDs) + "\n\n" + interactive.Title("Tenants") + "\n" + interactiveDetailList(item.TenantIDs)
	return []interactive.DetailTab{{Title: "Overview", Content: overview}, {Title: "Scope", Content: scope}}
}

func interactiveDetailFields(fields ...interactiveDetailField) string {
	var builder strings.Builder
	for _, field := range fields {
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(interactive.Label(fmt.Sprintf("%-13s", field.label)))
		builder.WriteString("  ")
		builder.WriteString(emptyInteractiveValue(field.value))
	}
	return builder.String()
}

func interactiveDetailList(values []string) string {
	if len(values) == 0 {
		return interactive.Muted("None")
	}
	return strings.Join(values, "\n")
}

func interactiveDetailMap(values map[string]string) string {
	if len(values) == 0 {
		return interactive.Muted("None")
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+values[key])
	}
	return strings.Join(lines, "\n")
}

func emptyInteractiveValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func interactiveTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04:05 MST")
}

func workspaceInteractiveRefresh() interactive.RefreshFunc {
	return func(context.Context) ([]interactive.Row, error) {
		manager := workspace.NewManager(workspace.DefaultStorePath())
		items, err := manager.List()
		if err != nil {
			return nil, err
		}
		return workspaceInteractiveRows(items), nil
	}
}
