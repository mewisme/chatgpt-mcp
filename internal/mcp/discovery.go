package mcp

import "go.mewis.me/chatgpt-mcp/internal/version"

const (
	SupportedProtocolVersion = "2026-07-28"
	defaultCacheTTLMS        = 0
	defaultCacheScope        = "private"
	ServerInstructions       = "Use chatgpt-mcp for local, workspace-aware coding and project operations. For project work, obtain a workspace_id with workspace_register unless one is already provided; use workspace_status to inspect its registered root, persisted shell cwd, and allowed directories. Filesystem and git paths resolve from the registered workspace root; shell commands use the server-side persisted shell cwd. Before substantial work in a workspace, call agent_status and project_context, discover skills with list_skills, and load each applicable skill with load_skill before acting. Load path-scoped rules with load_path_rules before modifying relevant files. Read files before editing and prefer apply_patch, edit_file, or multi_edit for deterministic changes. Use rewind checkpoints for inspection or recovery. Never assume project instructions, rules, or skills are already in context; load them on demand."
)

func serverInfo() map[string]any {
	return map[string]any{"name": "chatgpt-mcp", "version": version.Version}
}

func Discover() map[string]any {
	return map[string]any{
		"supportedVersions": []string{SupportedProtocolVersion},
		"capabilities":      DefaultCapabilities(),
		"instructions":      ServerInstructions,
		"ttlMs":             defaultCacheTTLMS,
		"cacheScope":        defaultCacheScope,
		"resultType":        "complete",
		"_meta":             map[string]any{"io.modelcontextprotocol/serverInfo": serverInfo()},
	}
}
