package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
		rows = append(rows, interactive.Row{ID: item.ID, Title: item.ID, Description: item.Path, Meta: meta, Summary: fmt.Sprintf("%-18s %s", item.ID, item.Path), Detail: prettyInteractiveJSON(item), Search: strings.Join(append(append([]string{}, item.AllowDirs...), item.LegacyIDs...), " ")})
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
		rows = append(rows, interactive.Row{ID: view.ID, Title: view.ID, Description: endpoint, Meta: strings.Join([]string{view.Transport, state, "expose " + view.Expose}, " · "), Summary: summary, Detail: prettyInteractiveJSON(view), Search: strings.Join([]string{view.Name, view.Transport, view.Expose, endpoint}, " ")})
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
		rows = append(rows, interactive.Row{ID: item.ID, Title: item.ID, Description: description, Meta: meta, Summary: summary, Detail: prettyInteractiveJSON(item), Search: strings.Join([]string{item.Name, item.Transport, string(item.Health), item.Expose, item.LastError}, " ")})
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
		rows = append(rows, interactive.Row{ID: item.ID, Title: title, Description: description, Meta: scope, Summary: summary, Detail: prettyInteractiveJSON(item), Search: strings.Join([]string{item.Name, item.Description, item.Creator, scope}, " ")})
	}
	return rows
}

func prettyInteractiveJSON(value any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return ""
	}
	return strings.TrimSuffix(buffer.String(), "\n")
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
