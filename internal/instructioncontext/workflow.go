package instructioncontext

const (
	agentWorkflowIntroduction = "Use chatgpt-mcp as a workspace-bound coding agent."
	serverIntroduction        = "Use chatgpt-mcp for local, workspace-aware coding and project operations."
	serverWorkspaceBootstrap  = "For project work, obtain a workspace_id with workspace_register unless one is already provided; use workspace_status to inspect its registered root, persisted shell cwd, and allowed directories."
	serverContextBootstrap    = "Before substantial workspace work, call agent_status and project_context. Use list_skills when skill summaries need to be discovered independently from project_context."

	guidanceWorkspace = "Stay inside the workspace selected by the current MCP session. The first valid workspace-scoped call binds the session; never switch that session to another workspace."
	guidanceContext   = "Treat project_context as the workspace instruction bundle. Follow project/user instructions and unconditional rules from it before acting."
	guidanceRead      = "Inspect relevant files before changing them. Use read_files/read_text_file for source context and load_path_rules for path-scoped rules before modifying matching files."
	guidanceSkills    = "Review the skill summaries in project_context. When a skill is applicable, call load_skill with its exact name before using that workflow."
	guidanceEdit      = "Prefer deterministic edits with apply_patch, edit_file, or multi_edit. Use run_command for commands, builds, tests, formatting, and other shell operations within the persisted workspace cwd."
	guidanceVerify    = "For non-trivial work, make a short plan, implement incrementally, and verify with the repository's relevant tests, lint, typecheck, build, or other documented checks."
	guidanceRewind    = "Use rewind to inspect or recover automatic file checkpoints when an edit must be reviewed or reverted."
	guidanceRemember  = "Use remember only for durable workspace-specific notes that will help future sessions; do not store secrets, transient status, or raw MCP session identifiers."
	guidanceMissing   = "Do not assume instructions, rules, skill bodies, Git state, or environment details that are absent from the supplied context. Query the appropriate tool instead of guessing."
	guidanceScope     = "Preserve unrelated user changes and keep mutations scoped to the requested task."

	DefaultAgentWorkflow = agentWorkflowIntroduction + "\n\n" +
		"1. " + guidanceWorkspace + "\n" +
		"2. " + guidanceContext + "\n" +
		"3. " + guidanceRead + "\n" +
		"4. " + guidanceSkills + "\n" +
		"5. " + guidanceEdit + "\n" +
		"6. " + guidanceVerify + "\n" +
		"7. " + guidanceRewind + "\n" +
		"8. " + guidanceRemember + "\n" +
		"9. " + guidanceMissing + "\n" +
		"10. " + guidanceScope

	defaultServerInstructions = serverIntroduction + " " + serverWorkspaceBootstrap + " " + serverContextBootstrap + " " +
		guidanceWorkspace + " " + guidanceContext + " " + guidanceRead + " " + guidanceSkills + " " + guidanceEdit + " " +
		guidanceVerify + " " + guidanceRewind + " " + guidanceRemember + " " + guidanceMissing + " " + guidanceScope
)

var sharedGuidanceSteps = []string{
	guidanceWorkspace,
	guidanceContext,
	guidanceRead,
	guidanceSkills,
	guidanceEdit,
	guidanceVerify,
	guidanceRewind,
	guidanceRemember,
	guidanceMissing,
	guidanceScope,
}

func AgentWorkflow() string {
	return DefaultAgentWorkflow
}

func SharedGuidanceSteps() []string {
	return append([]string(nil), sharedGuidanceSteps...)
}

func StaticServerInstructions() string {
	return defaultServerInstructions
}
