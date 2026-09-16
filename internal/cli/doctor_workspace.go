package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/instructioncontext"
	"go.mewis.me/chatgpt-mcp/internal/projectcontext"
	"go.mewis.me/chatgpt-mcp/internal/redact"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
)

func (d *doctorState) workspaceChecks() []doctorCheck {
	checks := []doctorCheck{
		{ID: "workspace.registry", Label: "workspace registry", Section: "Workspaces", Requires: []string{"config.source"}, Run: d.checkWorkspaceRegistry},
		{ID: "workspace.containers", Label: "workspace containers", Section: "Workspaces", Requires: []string{"workspace.registry"}, Run: d.checkWorkspaceContainers},
	}
	manager := workspace.NewManager(workspace.DefaultStorePath())
	items, err := manager.List()
	if err != nil {
		return checks
	}
	for _, item := range items {
		ws := item
		label := workspaceLabel(ws)
		checks = append(checks, doctorCheck{
			ID: "workspace." + ws.ID + ".identity", Label: label, Section: "Workspaces", Requires: []string{"workspace.registry"},
			Run: func(context.Context) doctorResult { return checkWorkspaceIdentity(ws) },
		})
		if strings.TrimSpace(ws.Error) != "" {
			continue
		}
		checks = append(checks, doctorCheck{
			ID: "workspace." + ws.ID + ".projectcontext", Label: label + " context", Section: "Workspaces",
			Requires: []string{"workspace." + ws.ID + ".identity"}, Timeout: 3 * time.Second,
			Run: func(ctx context.Context) doctorResult { return checkWorkspaceProjectContext(ctx, manager, ws) },
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
		return doctorResult{Status: doctorFail, Summary: "workspace " + workspaceLabel(ws) + " is unavailable", Error: redact.Text(ws.Error), Details: []string{ws.Path, ws.ID}}
	}
	info, err := os.Stat(ws.Path)
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "workspace path is unavailable", Error: redact.Text(err.Error()), Details: []string{ws.Path, ws.ID}}
	}
	if !info.IsDir() {
		return doctorResult{Status: doctorFail, Summary: "workspace path is not a directory", Details: []string{ws.Path, ws.ID}}
	}
	identity, err := workspacestate.Store{WorkspaceRoot: ws.Path}.LoadIdentity()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
			return doctorResult{Status: doctorFail, Summary: "workspace .cgm identity is missing", Details: []string{ws.Path, ws.ID}}
		}
		return doctorResult{Status: doctorFail, Summary: "workspace .cgm identity is invalid", Error: redact.Text(err.Error()), Details: []string{ws.Path, ws.ID}}
	}
	if identity.ID != ws.ID {
		return doctorResult{Status: doctorFail, Summary: "workspace identity mismatch", Details: []string{"registry " + ws.ID, "local " + identity.ID, ws.Path}}
	}
	return doctorResult{Status: doctorPass, Summary: "workspace identity is consistent", Details: []string{ws.Path, ws.ID}}
}

func checkWorkspaceProjectContext(ctx context.Context, manager *workspace.Manager, ws workspace.Workspace) doctorResult {
	store := workspacestate.Store{WorkspaceRoot: ws.Path}
	for _, path := range []string{store.RulesRoot(), store.SkillsRoot()} {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return doctorResult{Status: doctorFail, Summary: "project context source is inaccessible", Error: redact.Text(err.Error()), Details: []string{path, ws.ID}}
		}
		if !info.IsDir() {
			return doctorResult{Status: doctorFail, Summary: "project context source is not a directory", Details: []string{path, ws.ID}}
		}
	}
	opts := projectcontext.DefaultOptions()
	opts.IncludeGit = false
	result, err := projectcontext.New(manager, func() instructioncontext.ToolProfile {
		return instructioncontext.ToolProfile{Name: "full"}
	}).Build(ctx, ws.ID, opts)
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "project context preflight failed", Error: redact.Text(err.Error()), Details: []string{ws.Path, ws.ID}}
	}
	return doctorResult{Status: doctorPass, Summary: fmt.Sprintf("project context loaded (%d rules, %d skills)", result.Summary.Rules, result.Summary.Skills), Details: []string{ws.ID}}
}

func (d *doctorState) checkToolsRegistry(ctx context.Context) doctorResult {
	registry := tools.NewRegistry()
	workspaces := workspace.NewManager(workspace.DefaultStorePath())
	tools.RegisterCore(registry, workspaces, checkpoint.NewStore(config.RootPath()))
	count := len(registry.ListSchemas())
	if count == 0 {
		return doctorResult{Status: doctorFail, Summary: "core tool registry is empty"}
	}
	return doctorResult{Status: doctorPass, Summary: fmt.Sprintf("core tool registry has %d tools", count)}
}

func (d *doctorState) checkRuntimeJournal(ctx context.Context) doctorResult {
	root := config.RootPath()
	path := runtimeevent.Path(root)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return doctorResult{Status: doctorSkip, Summary: "runtime journal has not been created yet", Details: []string{path}}
	}
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "runtime journal is inaccessible", Error: redact.Text(err.Error()), Details: []string{path}}
	}
	if info.IsDir() {
		return doctorResult{Status: doctorFail, Summary: "runtime journal path is a directory", Details: []string{path}}
	}
	events, err := runtimeevent.Read(root, runtimeevent.Query{Tail: 20})
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "runtime journal could not be read", Error: redact.Text(err.Error()), Details: []string{path}}
	}
	return doctorResult{Status: doctorPass, Summary: fmt.Sprintf("runtime journal readable (%d recent events)", len(events)), Details: []string{path}}
}

func workspaceLabel(ws workspace.Workspace) string {
	base := filepath.Base(strings.TrimSpace(ws.Path))
	if base != "" && base != "." && base != string(filepath.Separator) {
		return base
	}
	return ws.ID
}
