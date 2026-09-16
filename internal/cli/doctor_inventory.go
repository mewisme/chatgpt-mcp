package cli

// doctorCoverage maps a runtime struct field to doctor check IDs or a parent check.
// Fields with no meaningful standalone probe must set Parent.
type doctorCoverage struct {
	Checks []string
	Parent string
	Note   string
}

// doctorAppCoverage is the Phase 1 inventory for app.App. Update this when bootstrap fields change.
var doctorAppCoverage = map[string]doctorCoverage{
	"Config":        {Checks: []string{"config.source", "config.integrity", "config.validate", "config.security"}},
	"MCP":           {Checks: []string{"network.mcp"}},
	"Upstream":      {Checks: []string{"upstream.store", "upstream.health"}},
	"Tools":         {Parent: "tools.registry", Note: "tools.Runtime fields have their own inventory"},
	"Activity":      {Parent: "observability.events"},
	"Tunnels":       {Checks: []string{"tunnel.collection", "plugin.secure-mcp-tunnel"}},
	"Tunnel":        {Parent: "tunnel.collection", Note: "legacy singleton adapter"},
	"Bridge":        {Parent: "plugin.secure-mcp-tunnel", Note: "private plugin MCP bridge"},
	"Logger":        {Checks: []string{"observability.events"}},
	"Notifications": {Checks: []string{"notification.provider"}},
	"runtimeCtx":    {Parent: "runtime.control"},
	"trace":         {Parent: "observability.events"},
	"running":       {Parent: "runtime.control"},
	"bootstrap":     {Parent: "runtime.control"},
	"bootstrapErr":  {Parent: "runtime.control"},
}

// doctorToolsCoverage is the Phase 1 inventory for tools.Runtime.
var doctorToolsCoverage = map[string]doctorCoverage{
	"Registry":         {Checks: []string{"tools.registry"}},
	"Workspaces":       {Checks: []string{"workspace.registry", "workspace.identity"}},
	"Checkpoints":      {Parent: "workspace.registry"},
	"Upstream":         {Checks: []string{"upstream.health"}},
	"CallObserver":     {Parent: "tools.registry"},
	"SessionAccess":    {Parent: "workspace.registry"},
	"Approvals":        {Parent: "notification.provider", Note: "approval health is review-path plus runtime bootstrap"},
	"Executions":       {Parent: "shell.provider"},
	"Hooks":            {Parent: "plugin.lock"},
	"PluginStore":      {Checks: []string{"plugin.lock", "plugin.desired", "plugin.capabilities", "plugin.registry", "plugin.payloads", "plugin.compatibility", "plugin.host", "plugin.admin-ui", "plugin.secure-mcp-tunnel"}},
	"Shell":            {Checks: []string{"shell.provider"}},
	"Processes":        {Parent: "shell.provider"},
	"LoopGuard":        {Parent: "tools.registry"},
	"PluginReconcile":  {Parent: "plugin.lock"},
	"WorkspacePlugins": {Checks: []string{"plugin.workspace"}},
	"sessionMu":        {Parent: "plugin.lock"},
	"pluginSessions":   {Parent: "plugin.lock"},
}

// doctorStandaloneChecks are subsystems not represented as App/tools.Runtime fields.
var doctorStandaloneChecks = []string{
	"install.current",
	"service.user",
	"service.system",
	"runtime.control",
	"network.plan",
	"network.admin",
	"auth.mcp",
	"auth.admin",
	"projectcontext.preflight",
	"cf-tunnel.mcp",
	"cf-tunnel.admin",
	"update.metadata",
}
