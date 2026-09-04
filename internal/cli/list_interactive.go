package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/cli/interactive"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func runInteractiveBrowser(cmd *cobra.Command, title string, rows []interactive.Row, refresh interactive.RefreshFunc) error {
	model := interactive.NewBrowser(cmd.Context(), title, rows, refresh)
	_, err := interactive.Run(cmd.Context(), model, cmd.InOrStdin(), cmd.OutOrStdout())
	return err
}

func workspaceInteractiveRows(items []workspace.Workspace) []interactive.Row {
	rows := make([]interactive.Row, 0, len(items))
	for _, item := range items {
		rows = append(rows, interactive.Row{ID: item.ID, Summary: fmt.Sprintf("%-18s %s", item.ID, item.Path), Detail: prettyInteractiveJSON(item), Search: strings.Join(item.AllowDirs, " ")})
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
		summary := fmt.Sprintf("%-18s %-6s enabled=%t expose=%s endpoint=%s", view.ID, view.Transport, view.Enabled, view.Expose, endpoint)
		rows = append(rows, interactive.Row{ID: view.ID, Summary: summary, Detail: prettyInteractiveJSON(view), Search: strings.Join([]string{view.Name, view.Transport, view.Expose, endpoint}, " ")})
	}
	return rows
}

func upstreamStatusInteractiveRows(items []upstream.Status) []interactive.Row {
	rows := make([]interactive.Row, 0, len(items))
	for _, item := range items {
		summary := fmt.Sprintf("%-18s %-6s enabled=%t health=%s tools=%d expose=%s", item.ID, item.Transport, item.Enabled, item.Health, item.ToolCount, item.Expose)
		rows = append(rows, interactive.Row{ID: item.ID, Summary: summary, Detail: prettyInteractiveJSON(item), Search: strings.Join([]string{item.Name, item.Transport, string(item.Health), item.Expose, item.LastError}, " ")})
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
		summary := fmt.Sprintf("%-22s %-24s scope=%s", item.ID, item.Name, scope)
		rows = append(rows, interactive.Row{ID: item.ID, Summary: summary, Detail: prettyInteractiveJSON(item), Search: strings.Join([]string{item.Name, item.Description, item.Creator, scope}, " ")})
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
