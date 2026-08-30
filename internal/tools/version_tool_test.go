package tools

import (
	"context"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	"go.mewis.me/chatgpt-mcp/internal/version"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestGetVersionTool(t *testing.T) {
	oldVersion, oldCommit, oldDate := version.Version, version.Commit, version.Date
	defer func() { version.Version, version.Commit, version.Date = oldVersion, oldCommit, oldDate }()
	version.Version, version.Commit, version.Date = "0.0.7", "abc123", "2026-08-30T19:26:57Z"
	registry := NewRegistry()
	RegisterCore(registry, workspace.NewManager(t.TempDir()+"/workspaces.json"), checkpoint.NewStore(t.TempDir()))
	schema, ok := registry.Schema("get_version")
	if !ok || schema.Annotations["readOnlyHint"] != true {
		t.Fatalf("get_version schema = %#v", schema)
	}
	result, err := registry.Call(context.Background(), "get_version", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := result.StructuredContent.(VersionResult)
	if !ok {
		t.Fatalf("structured content = %#v", result.StructuredContent)
	}
	if got.Version != "0.0.7" || got.Commit != "abc123" || got.BuildTime != "2026-08-30T19:26:57Z" {
		t.Fatalf("version result = %#v", got)
	}
}
