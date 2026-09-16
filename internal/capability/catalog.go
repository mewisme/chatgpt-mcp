package capability

import "strings"

type ID string

type Spec struct {
	ID            ID
	CanonicalPath string
	PublicPaths   []string
}

const RootPath = "<root>"

const (
	ServerForeground         ID = "server.foreground"
	InstallRun               ID = "install.run"
	InstallCleanup           ID = "install.cleanup"
	UpdateApply              ID = "update.apply"
	UpdateCheck              ID = "update.check"
	ConfigInit               ID = "config.init"
	ConfigUninit             ID = "config.uninit"
	RuntimeUp                ID = "runtime.up"
	RuntimeDown              ID = "runtime.down"
	RuntimeRestart           ID = "runtime.restart"
	LogsRead                 ID = "logs.read"
	LogsFollow               ID = "logs.follow"
	LogsPath                 ID = "logs.path"
	LogsClear                ID = "logs.clear"
	RequestList              ID = "request.list"
	RequestView              ID = "request.view"
	RequestApprove           ID = "request.approve"
	RequestDeny              ID = "request.deny"
	RequestGrantList         ID = "request.grant.list"
	RequestGrantRevoke       ID = "request.grant.revoke"
	ConfigPath               ID = "config.path"
	ConfigExport             ID = "config.export"
	ConfigImport             ID = "config.import"
	ConfigGet                ID = "config.get"
	ConfigList               ID = "config.list"
	ConfigSet                ID = "config.set"
	ConfigMigrate            ID = "config.migrate"
	ConfigMigrateSecrets     ID = "config.migrate.secrets"
	ConfigConvert            ID = "config.convert"
	ConfigVerify             ID = "config.verify"
	AliasInstall             ID = "alias.install"
	AliasRemove              ID = "alias.remove"
	AliasStatus              ID = "alias.status"
	AuthMCPRotate            ID = "auth.mcp.rotate"
	AuthMCPShow              ID = "auth.mcp.show"
	AuthMCPCopy              ID = "auth.mcp.copy"
	AuthMCPEnable            ID = "auth.mcp.enable"
	AuthMCPDisable           ID = "auth.mcp.disable"
	AuthAdminRotate          ID = "auth.admin.rotate"
	AuthAdminEnable          ID = "auth.admin.enable"
	AuthAdminDisable         ID = "auth.admin.disable"
	AuthStatus               ID = "auth.status"
	WorkspaceContainerList   ID = "workspace.container.list"
	WorkspaceContainerCreate ID = "workspace.container.create"
	WorkspaceContainerShow   ID = "workspace.container.show"
	WorkspaceContainerRename ID = "workspace.container.rename"
	WorkspaceContainerDelete ID = "workspace.container.delete"
	WorkspaceContainerAdd    ID = "workspace.container.add"
	WorkspaceContainerRemove ID = "workspace.container.remove"
	WorkspaceAccessList      ID = "workspace.access.list"
	WorkspaceAccessAdd       ID = "workspace.access.add"
	WorkspaceAccessRemove    ID = "workspace.access.remove"
	WorkspaceRegister        ID = "workspace.register"
	WorkspaceList            ID = "workspace.list"
	WorkspaceShow            ID = "workspace.show"
	WorkspaceRelocate        ID = "workspace.relocate"
	WorkspaceUnregister      ID = "workspace.unregister"
	WorkspacePurge           ID = "workspace.purge"
	MCPStdio                 ID = "mcp.stdio"
	MCPHTTP                  ID = "mcp.http"
	MCPServerList            ID = "mcp.server.list"
	MCPServerAdd             ID = "mcp.server.add"
	MCPServerConfigure       ID = "mcp.server.configure"
	MCPServerShow            ID = "mcp.server.show"
	MCPServerRemove          ID = "mcp.server.remove"
	MCPServerEnable          ID = "mcp.server.enable"
	MCPServerDisable         ID = "mcp.server.disable"
	MCPServerStatus          ID = "mcp.server.status"
	MCPServerTools           ID = "mcp.server.tools"
	PluginSearch             ID = "plugin.search"
	PluginInfo               ID = "plugin.info"
	PluginList               ID = "plugin.list"
	PluginInstall            ID = "plugin.install"
	PluginUninstall          ID = "plugin.uninstall"
	PluginEnable             ID = "plugin.enable"
	PluginDisable            ID = "plugin.disable"
	PluginUpdate             ID = "plugin.update"
	PluginRollback           ID = "plugin.rollback"
	PluginPrune              ID = "plugin.prune"
	PluginOutdated           ID = "plugin.outdated"
	PluginVerify             ID = "plugin.verify"
	PluginRegistryList       ID = "plugin.registry.list"
	PluginRegistryAdd        ID = "plugin.registry.add"
	PluginRegistryRemove     ID = "plugin.registry.remove"
	PluginConfigList         ID = "plugin.config.list"
	PluginConfigGet          ID = "plugin.config.get"
	PluginConfigSet          ID = "plugin.config.set"
	PluginConfigReset        ID = "plugin.config.reset"
	TunnelStatus             ID = "tunnel.status"
	TunnelAttach             ID = "tunnel.attach"
	TunnelAdd                ID = "tunnel.add"
	TunnelDetach             ID = "tunnel.detach"
	TunnelEnable             ID = "tunnel.enable"
	TunnelDisable            ID = "tunnel.disable"
	TunnelStart              ID = "tunnel.start"
	TunnelStop               ID = "tunnel.stop"
	TunnelForeground         ID = "tunnel.foreground"
	TunnelList               ID = "tunnel.list"
	TunnelManagedList        ID = "tunnel.managed.list"
	TunnelManagedGet         ID = "tunnel.managed.get"
	TunnelManagedCreate      ID = "tunnel.managed.create"
	TunnelManagedUpdate      ID = "tunnel.managed.update"
	TunnelManagedDelete      ID = "tunnel.managed.delete"
	TunnelAdminList          ID = "tunnel.admin.list"
	TunnelAdminAdd           ID = "tunnel.admin.add"
	TunnelAdminUpdate        ID = "tunnel.admin.update"
	TunnelAdminVerify        ID = "tunnel.admin.verify"
	TunnelAdminRemove        ID = "tunnel.admin.remove"
	TunnelUpdate             ID = "tunnel.update"
	StatusOverview           ID = "status.overview"
	VersionAbout             ID = "version.about"
)

var specs = []Spec{
	{ServerForeground, "serve", []string{RootPath}},
	{InstallRun, "install", nil}, {InstallCleanup, "install cleanup", nil},
	{UpdateApply, "upgrade", nil}, {UpdateCheck, "upgrade check", nil},
	{ConfigInit, "init", nil}, {ConfigUninit, "uninit", nil},
	{RuntimeUp, "up", nil}, {RuntimeDown, "down", nil}, {RuntimeRestart, "restart", nil},
	{LogsRead, "logs", nil}, {LogsFollow, "logs follow", nil}, {LogsPath, "logs path", nil}, {LogsClear, "logs clear", nil},
	{RequestList, "request list", nil}, {RequestView, "request view", nil}, {RequestApprove, "request approve", nil}, {RequestDeny, "request deny", nil},
	{RequestGrantList, "request grant list", nil}, {RequestGrantRevoke, "request grant revoke", nil},
	{ConfigPath, "config path", nil}, {ConfigExport, "config export", nil}, {ConfigImport, "config import", nil},
	{ConfigGet, "config get", []string{"config explain"}}, {ConfigList, "config list", nil}, {ConfigSet, "config set", nil}, {ConfigMigrate, "config migrate", nil},
	{ConfigMigrateSecrets, "config migrate secrets", nil}, {ConfigConvert, "config convert", nil}, {ConfigVerify, "config verify", nil},
	{AliasInstall, "alias install", nil}, {AliasRemove, "alias remove", nil}, {AliasStatus, "alias status", nil},
	{AuthMCPRotate, "auth mcp rotate", []string{"auth mcp create"}}, {AuthMCPShow, "auth mcp show", nil}, {AuthMCPCopy, "auth mcp copy", nil}, {AuthMCPEnable, "auth mcp enable", nil}, {AuthMCPDisable, "auth mcp disable", nil},
	{AuthAdminRotate, "auth admin create", nil}, {AuthAdminEnable, "auth admin enable", nil}, {AuthAdminDisable, "auth admin disable", nil}, {AuthStatus, "auth status", []string{"auth mcp status"}},
	{WorkspaceContainerList, "workspace container list", nil}, {WorkspaceContainerCreate, "workspace container create", nil}, {WorkspaceContainerShow, "workspace container show", nil},
	{WorkspaceContainerRename, "workspace container rename", nil}, {WorkspaceContainerDelete, "workspace container delete", nil}, {WorkspaceContainerAdd, "workspace container add", nil}, {WorkspaceContainerRemove, "workspace container remove", nil},
	{WorkspaceAccessList, "workspace access list", nil}, {WorkspaceAccessAdd, "workspace access add", nil}, {WorkspaceAccessRemove, "workspace access remove", nil},
	{WorkspaceRegister, "workspace register", nil}, {WorkspaceList, "workspace list", nil}, {WorkspaceShow, "workspace show", nil}, {WorkspaceRelocate, "workspace relocate", nil}, {WorkspaceUnregister, "workspace unregister", nil}, {WorkspacePurge, "workspace purge", nil},
	{MCPStdio, "mcp stdio", nil},
	{MCPHTTP, "mcp http", nil},
	{MCPServerList, "upstream server list", []string{"mcp server list"}}, {MCPServerAdd, "upstream server add", []string{"mcp server add"}}, {MCPServerConfigure, "upstream server configure", []string{"mcp server configure"}}, {MCPServerShow, "upstream server show", []string{"mcp server show"}},
	{MCPServerRemove, "upstream server remove", []string{"mcp server remove"}}, {MCPServerEnable, "upstream server enable", []string{"mcp server enable"}}, {MCPServerDisable, "upstream server disable", []string{"mcp server disable"}}, {MCPServerStatus, "upstream server status", []string{"mcp server status"}}, {MCPServerTools, "upstream server tools", []string{"mcp server tools"}},
	{PluginSearch, "plugin search", nil}, {PluginInfo, "plugin info", nil}, {PluginList, "plugin list", nil}, {PluginInstall, "plugin install", nil}, {PluginUninstall, "plugin uninstall", nil},
	{PluginEnable, "plugin enable", nil}, {PluginDisable, "plugin disable", nil}, {PluginUpdate, "plugin update", nil}, {PluginRollback, "plugin rollback", nil}, {PluginPrune, "plugin prune", nil},
	{PluginOutdated, "plugin outdated", nil}, {PluginVerify, "plugin verify", nil},
	{PluginRegistryList, "plugin registry list", nil}, {PluginRegistryAdd, "plugin registry add", nil}, {PluginRegistryRemove, "plugin registry remove", nil},
	{PluginConfigList, "plugin config list", nil}, {PluginConfigGet, "plugin config get", nil}, {PluginConfigSet, "plugin config set", nil}, {PluginConfigReset, "plugin config reset", nil},
	{TunnelStatus, "tunnel status", nil}, {TunnelList, "tunnel list", nil}, {TunnelAttach, "tunnel attach", nil}, {TunnelAdd, "tunnel add", nil}, {TunnelUpdate, "tunnel update", nil}, {TunnelDetach, "tunnel detach", nil}, {TunnelEnable, "tunnel enable", nil}, {TunnelDisable, "tunnel disable", nil}, {TunnelStart, "tunnel start", nil}, {TunnelStop, "tunnel stop", nil}, {TunnelForeground, "tunnel run", nil},
	{TunnelManagedList, "tunnel managed list", nil}, {TunnelManagedGet, "tunnel managed get", nil}, {TunnelManagedCreate, "tunnel managed create", nil}, {TunnelManagedUpdate, "tunnel managed update", nil}, {TunnelManagedDelete, "tunnel managed delete", nil},
	{TunnelAdminList, "tunnel admin list", nil}, {TunnelAdminAdd, "tunnel admin add", nil}, {TunnelAdminUpdate, "tunnel admin update", nil}, {TunnelAdminVerify, "tunnel admin verify", nil}, {TunnelAdminRemove, "tunnel admin remove", nil},
	{StatusOverview, "status", nil}, {VersionAbout, "version", nil},
}

func All() []Spec { return append([]Spec(nil), specs...) }

func Lookup(id ID) (Spec, bool) {
	for _, spec := range specs {
		if spec.ID == id {
			return spec, true
		}
	}
	return Spec{}, false
}

func ForPath(path string) (ID, bool) {
	path = NormalizePath(path)
	for _, spec := range specs {
		if NormalizePath(spec.CanonicalPath) == path {
			return spec.ID, true
		}
		for _, candidate := range spec.PublicPaths {
			if NormalizePath(candidate) == path {
				return spec.ID, true
			}
		}
	}
	return "", false
}

func SearchText(ids []ID) string {
	parts := make([]string, 0, len(ids)*2)
	for _, id := range ids {
		if spec, ok := Lookup(id); ok {
			parts = append(parts, spec.CanonicalPath)
			parts = append(parts, spec.PublicPaths...)
		}
	}
	return strings.Join(parts, " ")
}

func NormalizePath(path string) string {
	if strings.TrimSpace(path) == RootPath {
		return RootPath
	}
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(path)))
	kept := fields[:0]
	for _, field := range fields {
		if strings.HasPrefix(field, "-") {
			continue
		}
		kept = append(kept, field)
	}
	return strings.Join(kept, " ")
}
