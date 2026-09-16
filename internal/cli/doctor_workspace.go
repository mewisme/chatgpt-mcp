package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/redact"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
)

func (d *doctorState) workspaceChecks() []doctorCheck {
	checks := []doctorCheck{
		{ID: "workspace.registry", Section: "Workspaces", Requires: []string{"config.source"}, Run: d.checkWorkspaceRegistry},
		{ID: "workspace.containers", Section: "Workspaces", Requires: []string{"workspace.registry"}, Run: d.checkWorkspaceContainers},
	}
	manager := workspace.NewManager(workspace.DefaultStorePath())
	items, err := manager.List()
	if err != nil {
		return checks
	}
	for _, item := range items {
		ws := item
		checks = append(checks, doctorCheck{
			ID: "workspace." + ws.ID + ".identity", Section: "Workspaces", Requires: []string{"workspace.registry"},
			Run: func(context.Context) doctorResult { return checkWorkspaceIdentity(ws) },
		})
	}
	return checks
}

func (d *doctorState) checkWorkspaceRegistry(ctx context.Context) doctorResult {
	manager := workspace.NewManager(workspace.DefaultStorePath())
	items, err := manager.List()
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "workspace registry could not be read", Error: redact.Text(err.Error())}
	}
	if len(items) == 0 {
		return doctorResult{Status: doctorPass, Summary: "no workspaces registered"}
	}
	return doctorResult{Status: doctorPass, Summary: fmt.Sprintf("%d workspace(s) registered", len(items))}
}

func (d *doctorState) checkWorkspaceContainers(ctx context.Context) doctorResult {
	manager := workspace.NewManager(workspace.DefaultStorePath())
	items, err := manager.ListContainers()
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "workspace containers could not be read", Error: redact.Text(err.Error())}
	}
	if len(items) == 0 {
		return doctorResult{Status: doctorSkip, Summary: "no workspace containers configured"}
	}
	return doctorResult{Status: doctorPass, Summary: fmt.Sprintf("%d workspace container(s) configured", len(items))}
}

func checkWorkspaceIdentity(ws workspace.Workspace) doctorResult {
	if strings.TrimSpace(ws.Error) != "" {
		return doctorResult{Status: doctorFail, Summary: "workspace " + ws.ID + " is unavailable", Error: redact.Text(ws.Error), Details: []string{ws.Path}}
	}
	info, err := os.Stat(ws.Path)
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "workspace path is unavailable", Error: redact.Text(err.Error()), Details: []string{ws.Path}}
	}
	if !info.IsDir() {
		return doctorResult{Status: doctorFail, Summary: "workspace path is not a directory", Details: []string{ws.Path}}
	}
	identity, err := workspacestate.Store{WorkspaceRoot: ws.Path}.LoadIdentity()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
			return doctorResult{Status: doctorFail, Summary: "workspace .cgm identity is missing", Details: []string{ws.Path}}
		}
		return doctorResult{Status: doctorFail, Summary: "workspace .cgm identity is invalid", Error: redact.Text(err.Error()), Details: []string{ws.Path}}
	}
	if identity.ID != ws.ID {
		return doctorResult{Status: doctorFail, Summary: "workspace identity mismatch", Details: []string{"registry " + ws.ID, "local " + identity.ID, ws.Path}}
	}
	return doctorResult{Status: doctorPass, Summary: "workspace identity is consistent", Details: []string{ws.Path}}
}
