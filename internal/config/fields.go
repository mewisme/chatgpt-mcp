package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

const RedactedValue = "<redacted>"

type FieldKind string

const (
	FieldBool     FieldKind = "bool"
	FieldInt      FieldKind = "int"
	FieldString   FieldKind = "string"
	FieldList     FieldKind = "list"
	FieldEnum     FieldKind = "enum"
	FieldReadOnly FieldKind = "readonly"
)

type FieldSpec struct {
	Key         string
	Label       string
	Section     FieldSection
	Description string
	Details     string
	Kind        FieldKind
	Options     []string
	Values      []FieldValueSpec
	Editable    bool
	Sensitive   bool
	Guidance    string
	Related     []string
}

type FieldValueSpec struct {
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

type FieldSection string

const (
	FieldSectionRuntime FieldSection = "runtime"
	FieldSectionAccess  FieldSection = "access"
	FieldSectionShell   FieldSection = "shell"
	FieldSectionTunnel  FieldSection = "tunnel"
)

type FieldState string

const (
	FieldStateDefault FieldState = "default"
	FieldStateCustom  FieldState = "custom"
	FieldStateManaged FieldState = "managed"
)

var fieldSpecs = []FieldSpec{
	{Key: "server.enabled", Label: "MCP HTTP server", Section: FieldSectionRuntime, Description: "controls whether the MCP HTTP transport is enabled", Details: "When disabled, clients cannot connect through the local HTTP MCP server. At least one MCP transport must remain enabled, so the Secure MCP Tunnel must be enabled before this can be disabled by itself.", Kind: FieldBool, Editable: true, Related: []string{"server.port", "server.expose.mode", "auth.mcp_enabled", "tunnel.enabled"}},
	{Key: "server.expose.mode", Label: "Exposure", Section: FieldSectionRuntime, Description: "controls which local network addresses expose the HTTP servers", Details: "Loopback access is always retained. Any non-loopback exposure requires server.allow_insecure_http=true and valid authentication for each enabled HTTP endpoint.", Kind: FieldEnum, Options: []string{"none", "all", "0.0.0.0", "interfaces"}, Values: []FieldValueSpec{{Value: "none", Description: "Bind only to loopback."}, {Value: "all", Description: "Bind loopback plus every eligible address discovered on all interfaces."}, {Value: "0.0.0.0", Description: "Bind one IPv4 wildcard listener and expose eligible IPv4 addresses."}, {Value: "interfaces", Description: "Bind loopback plus addresses from server.expose.interfaces."}}, Editable: true, Related: []string{"server.expose.interfaces", "server.allow_insecure_http", "auth.mcp_enabled", "auth.admin_enabled"}},
	{Key: "server.expose.interfaces", Label: "Exposure interfaces", Section: FieldSectionRuntime, Description: "lists network interfaces used when exposure mode is interfaces", Details: "Each name must resolve to an available interface with at least one eligible IP address at runtime. Duplicate names are removed and values are normalized before persistence.", Kind: FieldList, Editable: true, Guidance: "Set server.expose.mode=interfaces before relying on this list.", Related: []string{"server.expose.mode", "server.allow_insecure_http"}},
	{Key: "server.port", Label: "MCP HTTP port", Section: FieldSectionRuntime, Description: "sets the TCP port for the MCP HTTP server", Details: "Valid range is 1-65535. When both MCP and admin HTTP servers are enabled, their ports must differ.", Kind: FieldInt, Editable: true, Related: []string{"server.enabled", "admin.port"}},
	{Key: "server.allow_insecure_http", Label: "Allow insecure HTTP", Section: FieldSectionRuntime, Description: "allows authenticated plain HTTP endpoints beyond loopback", Details: "This opt-in is required for non-loopback exposure. It does not disable authentication requirements; exposed enabled endpoints still require configured credentials. Prefer the Secure MCP Tunnel or a TLS reverse proxy when possible.", Kind: FieldBool, Editable: true, Related: []string{"server.expose.mode", "auth.mcp_enabled", "auth.admin_enabled", "tunnel.enabled"}},
	{Key: "server.allow_unauthenticated_loopback", Label: "Allow unauthenticated loopback", Section: FieldSectionRuntime, Description: "WARNING: acknowledges intentionally disabling MCP/Admin HTTP authentication on loopback", Details: "Required before auth.mcp_enabled or auth.admin_enabled can be turned off while the corresponding HTTP server remains enabled. Valid only with server.expose.mode=none. Unauthenticated listeners accept any local process as a client; prefer keeping authentication enabled.", Kind: FieldBool, Editable: true, Guidance: "Set this only for trusted local development, then re-enable authentication promptly.", Related: []string{"auth.mcp_enabled", "auth.admin_enabled", "server.expose.mode", "server.enabled", "admin.enabled"}},
	{Key: "admin.enabled", Label: "Admin server", Section: FieldSectionRuntime, Description: "controls whether the admin HTTP server is enabled", Details: "When enabled, the admin endpoint listens using the configured admin port and the same network exposure policy. If admin authentication is enabled, a configured admin credential is required.", Kind: FieldBool, Editable: true, Related: []string{"admin.port", "auth.admin_enabled", "server.expose.mode"}},
	{Key: "admin.port", Label: "Admin port", Section: FieldSectionRuntime, Description: "sets the TCP port for the admin HTTP server", Details: "Valid range is 1-65535 while the admin server is enabled. When both HTTP servers are enabled, this port must differ from server.port.", Kind: FieldInt, Editable: true, Related: []string{"admin.enabled", "server.port"}},
	{Key: "auth.mcp_enabled", Label: "Direct MCP HTTP authentication", Section: FieldSectionAccess, Description: "controls token authentication for direct /mcp HTTP access", Details: "Protects direct connections to /mcp. Secure MCP Tunnel uses separate tunnel credentials and is unaffected. Reuse the Direct MCP HTTP token when adding this MCP server to ChatGPT; a new token is not required for each ChatGPT configuration. When the MCP HTTP server is enabled and this setting is true, a Direct MCP HTTP token must be configured. Disabling authentication while the MCP HTTP server remains enabled requires server.allow_unauthenticated_loopback=true and server.expose.mode=none. Non-loopback HTTP exposure always requires Direct MCP HTTP authentication with a configured token.", Kind: FieldBool, Editable: true, Related: []string{"auth.mcp_token_hash", "server.enabled", "server.expose.mode", "server.allow_unauthenticated_loopback"}},
	{Key: "auth.mcp_legacy_bearer", Label: "Legacy MCP bearer", Section: FieldSectionAccess, Description: "allows the existing static MCP token as a compatibility bearer credential", Details: "Leftover after inbound OAuth removal; Direct MCP HTTP uses the hashed bearer in auth.mcp_token_hash.", Kind: FieldBool, Editable: true, Related: []string{"auth.mcp_enabled", "auth.mcp_token_hash"}},
	{Key: "auth.admin_enabled", Label: "Admin authentication", Section: FieldSectionAccess, Description: "controls token authentication for the admin HTTP endpoint", Details: "When the admin server is enabled and this setting is true, an admin credential must be configured. Disabling authentication while the admin server remains enabled requires server.allow_unauthenticated_loopback=true and server.expose.mode=none. Non-loopback exposure with the admin endpoint enabled always requires admin authentication.", Kind: FieldBool, Editable: true, Related: []string{"auth.admin_token_hash", "admin.enabled", "server.expose.mode", "server.allow_unauthenticated_loopback"}},
	{Key: "auth.mcp_token_hash", Label: "Direct MCP HTTP token", Section: FieldSectionAccess, Description: "stores the managed credential hash for Direct MCP HTTP authentication", Details: "The raw token is never exposed through config views. Reuse this token when adding this MCP server to ChatGPT. You do not need to generate a new token for each connection. This field is managed by cgm auth mcp rotate and is not directly editable through config set.", Kind: FieldReadOnly, Sensitive: true, Guidance: "Reuse this token when adding this MCP server to ChatGPT. You do not need to generate a new token for each connection.", Related: []string{"auth.mcp_enabled", "server.enabled"}},
	{Key: "auth.admin_token_hash", Label: "Admin credential", Section: FieldSectionAccess, Description: "stores the managed credential hash used by admin HTTP authentication", Details: "The raw token is never exposed through config views. This field is managed by the admin authentication workflow and is not directly editable through config set.", Kind: FieldReadOnly, Sensitive: true, Guidance: "Manage this credential with the admin auth token workflow.", Related: []string{"auth.admin_enabled", "admin.enabled"}},
	{Key: "permissions.allow_dirs", Label: "Allowed directories", Section: FieldSectionAccess, Description: "adds global filesystem roots that registered workspaces may access", Details: "These roots extend workspace-local access for filesystem and shell operations. Paths must be absolute, are normalized, and apply globally in addition to per-workspace allowed directories.", Kind: FieldList, Editable: true},
	{Key: "shell.executable", Label: "Bash executable", Section: FieldSectionShell, Description: "selects an explicit Bash executable for managed shell commands", Details: "When set, this absolute Bash path has priority over the enabled shell/bash plugin and system Bash discovery. PowerShell is not a valid agent shell provider.", Kind: FieldString, Editable: true, Related: []string{"shell.path"}},
	{Key: "shell.path", Label: "Executable search paths", Section: FieldSectionShell, Description: "prepends additional executable directories to PATH for managed shell commands", Details: "Paths must be absolute. Configured entries are prepended to the inherited process PATH for foreground and background shell execution.", Kind: FieldList, Editable: true},
	{Key: "tunnel.enabled", Label: "Tunnel", Section: FieldSectionTunnel, Description: "reports whether any OpenAI Secure MCP Tunnel instance is enabled", Details: "This value is derived from the tunnel collection. Enable or disable a specific instance with cgm tunnel enable/disable.", Kind: FieldReadOnly, Related: []string{"tunnel.instances", "server.enabled"}},
	{Key: "tunnel.instances", Label: "Tunnel instances", Section: FieldSectionTunnel, Description: "lists configured runtime tunnel instances", Details: "Manage instances by tunnel ID; collection editing is not available through config set.", Kind: FieldReadOnly},
	{Key: "tunnel.admins", Label: "Tunnel admin profiles", Section: FieldSectionTunnel, Description: "lists configured management profiles", Details: "Manage profiles by profile ID; collection editing is not available through config set.", Kind: FieldReadOnly},
	{Key: "tunnel.id", Label: "Tunnel ID", Section: FieldSectionTunnel, Description: "identifies one local tunnel instance for compatibility views", Details: "Derived from the first collection instance or leftover scalar config. Attach or update tunnels with cgm tunnel add/update.", Kind: FieldReadOnly, Related: []string{"tunnel.instances", "tunnel.api_key"}},
	{Key: "tunnel.api_key", Label: "Runtime API key", Section: FieldSectionTunnel, Description: "stores the managed runtime credential used to connect to the Secure MCP Tunnel", Details: "The raw runtime key is stored through the secret workflow and is redacted from config views. Manage keys with cgm tunnel add/update.", Kind: FieldReadOnly, Sensitive: true, Guidance: "Manage the runtime key with cgm tunnel add/update.", Related: []string{"tunnel.instances", "tunnel.id"}},
	{Key: "tunnel.admin_key", Label: "Admin key", Section: FieldSectionTunnel, Description: "stores the managed admin credential used for tunnel control-plane operations", Details: "The admin key is separate from the runtime tunnel key. It is used for management operations such as listing, creating, updating, or deleting managed tunnels and is redacted from config views.", Kind: FieldReadOnly, Sensitive: true, Guidance: "Manage and verify the admin key with cgm tunnel admin.", Related: []string{"tunnel.admins", "tunnel.admin_organization_id", "tunnel.admin_workspace_id", "tunnel.admin_tenant_id"}},
	{Key: "tunnel.admin_organization_id", Label: "Admin organization scope", Section: FieldSectionTunnel, Description: "records the verified organization scope for the tunnel admin key", Details: "This read-only value is populated from admin-key verification and constrains tunnel management operations to the verified organization scope when present.", Kind: FieldReadOnly, Guidance: "This scope is set only after admin-key verification.", Related: []string{"tunnel.admin_key", "tunnel.admin_workspace_id", "tunnel.admin_tenant_id"}},
	{Key: "tunnel.admin_workspace_id", Label: "Admin workspace scope", Section: FieldSectionTunnel, Description: "records the verified workspace scope for the tunnel admin key", Details: "This read-only value is populated from admin-key verification and constrains tunnel management operations to the verified workspace scope when present.", Kind: FieldReadOnly, Guidance: "This scope is set only after admin-key verification.", Related: []string{"tunnel.admin_key", "tunnel.admin_organization_id", "tunnel.admin_tenant_id"}},
	{Key: "tunnel.admin_tenant_id", Label: "Admin tenant scope", Section: FieldSectionTunnel, Description: "records the verified tenant scope for the tunnel admin key", Details: "This read-only value is populated from admin-key verification and constrains tunnel management operations to the verified tenant scope when present.", Kind: FieldReadOnly, Guidance: "This scope is set only after admin-key verification.", Related: []string{"tunnel.admin_key", "tunnel.admin_organization_id", "tunnel.admin_workspace_id"}},
	{Key: "tunnel.control_plane_base_url", Label: "Control-plane URL", Section: FieldSectionTunnel, Description: "overrides the OpenAI tunnel control-plane base URL", Details: "Derived from the first collection instance or leftover scalar config. Set it with cgm tunnel add/update.", Kind: FieldReadOnly, Guidance: "Leave empty unless a different control-plane endpoint is explicitly required.", Related: []string{"tunnel.instances"}},
	{Key: "tunnel.organization_id", Label: "Organization ID", Section: FieldSectionTunnel, Description: "sets the OpenAI organization context associated with tunnel runtime operations", Details: "Derived from the first collection instance or leftover scalar config. Set it with cgm tunnel add/update.", Kind: FieldReadOnly, Related: []string{"tunnel.instances", "tunnel.admin_organization_id"}},
	{Key: "notifications.enabled", Label: "Desktop notifications", Section: FieldSectionRuntime, Description: "controls whether ChatGPT MCP may send desktop notifications", Details: "This is a best-effort host notification switch. Approval requests still work when notifications are disabled or the desktop provider is unavailable.", Kind: FieldBool, Editable: true, Related: []string{"notifications.approvals", "notifications.when_tui_inactive", "notifications.open_action"}},
	{Key: "notifications.approvals", Label: "Approval notifications", Section: FieldSectionRuntime, Description: "controls desktop alerts for pending control approval requests", Details: "When enabled together with notifications.enabled, a pending approval.requested event can produce a lock-screen-safe desktop notification. The notification never approves or denies the request.", Kind: FieldBool, Editable: true, Related: []string{"notifications.enabled", "notifications.when_tui_inactive", "notifications.open_action"}},
	{Key: "notifications.when_tui_inactive", Label: "Notify only without TUI", Section: FieldSectionRuntime, Description: "suppresses desktop approval notifications while a TUI reviewer is open", Details: "An open TUI holds a presence lock for this config root. When this setting is true, desktop notifications are skipped while that lock is held. A failed presence check prefers sending a notification rather than dropping the request silently.", Kind: FieldBool, Editable: true, Related: []string{"notifications.enabled", "notifications.approvals"}},
	{Key: "notifications.open_action", Label: "Notification open action", Section: FieldSectionRuntime, Description: "controls whether a notification may open the matching TUI request", Details: "auto exposes a Review action only when the host provider can reliably activate a terminal. disabled keeps notifications passive. v1 providers are passive because one-shot host tools cannot receive notification clicks.", Kind: FieldEnum, Options: []string{"auto", "disabled"}, Values: []FieldValueSpec{{Value: "auto", Description: "Offer Review only when the provider supports reliable activation."}, {Value: "disabled", Description: "Always send a passive notification with no open action."}}, Editable: true, Related: []string{"notifications.enabled", "notifications.approvals"}},
}

func Fields() []FieldSpec {
	result := make([]FieldSpec, len(fieldSpecs))
	for index, spec := range fieldSpecs {
		result[index] = cloneFieldSpec(spec)
	}
	return result
}

func FieldByKey(key string) (FieldSpec, bool) {
	key = canonicalFieldKey(key)
	for _, spec := range fieldSpecs {
		if spec.Key == key {
			return cloneFieldSpec(spec), true
		}
	}
	return FieldSpec{}, false
}

func cloneFieldSpec(spec FieldSpec) FieldSpec {
	spec.Options = append([]string(nil), spec.Options...)
	spec.Values = append([]FieldValueSpec(nil), spec.Values...)
	spec.Related = append([]string(nil), spec.Related...)
	return spec
}

func SetValue(cfg *Config, key, raw string) error {
	if cfg == nil {
		return errors.New("config is required")
	}
	key = canonicalFieldKey(key)
	switch key {
	case "server.enabled":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Server.Enabled = value
	case "server.expose":
		value, err := ParseExposure(raw)
		if err != nil {
			return err
		}
		cfg.Server.Expose = value
	case "server.expose.mode":
		mode := ExposureMode(strings.ToLower(strings.TrimSpace(raw)))
		if mode != ExposureNone && mode != ExposureAll && mode != ExposureWildcard && mode != ExposureInterfaces {
			return errors.New("server.expose.mode must be none, all, 0.0.0.0, or interfaces")
		}
		cfg.Server.Expose.Mode = mode
		cfg.Server.Expose = NormalizeExposure(cfg.Server.Expose)
	case "server.expose.interfaces":
		cfg.Server.Expose = NormalizeExposure(ExposureConfig{Mode: ExposureInterfaces, Interfaces: splitFieldList(raw)})
	case "server.port":
		value, err := parseIntField(raw, key)
		if err != nil {
			return err
		}
		cfg.Server.Port = value
	case "server.allow_insecure_http":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Server.AllowInsecureHTTP = value
	case "server.allow_unauthenticated_loopback":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Server.AllowUnauthenticatedLoopback = value
	case "admin.enabled":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Admin.Enabled = value
	case "admin.port":
		value, err := parseIntField(raw, key)
		if err != nil {
			return err
		}
		cfg.Admin.Port = value
	case "auth.mcp_enabled":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Auth.MCPEnabled = value
	case "auth.mcp_legacy_bearer":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Auth.MCPLegacyBearer = value
	case "auth.admin_enabled":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Auth.AdminEnabled = value
	case "permissions.allow_dirs":
		cfg.Permissions.AllowDirs = splitFieldList(raw)
	case "shell.executable":
		cfg.Shell.Executable = strings.TrimSpace(raw)
	case "shell.path":
		cfg.Shell.Path = splitFieldList(raw)
	case "tunnel.enabled", "tunnel.id", "tunnel.api_key", "tunnel.control_plane_base_url", "tunnel.organization_id":
		return errors.New("tunnel runtime fields cannot be set through config; use cgm tunnel add/update")
	case "tunnel.instances", "tunnel.admins":
		return errors.New("tunnel collections cannot be edited through config set")
	case "tunnel.admin_key", "tunnel.admin_organization_id", "tunnel.admin_workspace_id", "tunnel.admin_tenant_id":
		return errors.New("tunnel admin credentials cannot be set through config; use chatgpt-mcp tunnel admin key")
	case "notifications.enabled":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Notifications.Enabled = value
	case "notifications.approvals":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Notifications.Approvals = value
	case "notifications.when_tui_inactive":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Notifications.WhenTUIInactive = value
	case "notifications.open_action":
		value := strings.ToLower(strings.TrimSpace(raw))
		if value != "auto" && value != "disabled" {
			return errors.New("notifications.open_action must be auto or disabled")
		}
		cfg.Notifications.OpenAction = value
	case "auth.mcp_token_hash", "auth.admin_token_hash":
		return errors.New("token hashes cannot be set through config; use chatgpt-mcp auth <mcp|admin> create")
	default:
		return fmt.Errorf("unsupported config key: %s", key)
	}
	return nil
}

func SetValueValidated(cfg *Config, key, raw string) error {
	if cfg == nil {
		return errors.New("config is required")
	}
	next := *cfg
	if err := SetValue(&next, key, raw); err != nil {
		return err
	}
	if err := Validate(next); err != nil {
		return err
	}
	*cfg = next
	return nil
}

func RawValue(cfg Config, key string) (string, error) {
	key = canonicalFieldKey(key)
	switch key {
	case "server.enabled":
		return strconv.FormatBool(cfg.Server.Enabled), nil
	case "server.expose":
		exposure := NormalizeExposure(cfg.Server.Expose)
		if exposure.Mode == ExposureInterfaces {
			return strings.Join(exposure.Interfaces, ","), nil
		}
		return string(exposure.Mode), nil
	case "server.expose.mode":
		return string(NormalizeExposure(cfg.Server.Expose).Mode), nil
	case "server.expose.interfaces":
		return strings.Join(NormalizeExposure(cfg.Server.Expose).Interfaces, ","), nil
	case "server.port":
		return strconv.Itoa(cfg.Server.Port), nil
	case "server.allow_insecure_http":
		return strconv.FormatBool(cfg.Server.AllowInsecureHTTP), nil
	case "server.allow_unauthenticated_loopback":
		return strconv.FormatBool(cfg.Server.AllowUnauthenticatedLoopback), nil
	case "admin.enabled":
		return strconv.FormatBool(cfg.Admin.Enabled), nil
	case "admin.port":
		return strconv.Itoa(cfg.Admin.Port), nil
	case "auth.mcp_enabled":
		return strconv.FormatBool(cfg.Auth.MCPEnabled), nil
	case "auth.mcp_legacy_bearer":
		return strconv.FormatBool(cfg.Auth.MCPLegacyBearer), nil
	case "auth.admin_enabled":
		return strconv.FormatBool(cfg.Auth.AdminEnabled), nil
	case "auth.mcp_token_hash":
		return cfg.Auth.MCPTokenHash, nil
	case "auth.admin_token_hash":
		return cfg.Auth.AdminTokenHash, nil
	case "permissions.allow_dirs":
		return strings.Join(cfg.Permissions.AllowDirs, ","), nil
	case "shell.executable":
		return cfg.Shell.Executable, nil
	case "shell.path":
		return strings.Join(cfg.Shell.Path, ","), nil
	case "tunnel.enabled":
		return strconv.FormatBool(cfg.EnabledTunnelCount() > 0), nil
	case "tunnel.instances":
		data, err := json.Marshal(cfg.Tunnel.Collection().Instances)
		return string(data), err
	case "tunnel.admins":
		data, err := json.Marshal(cfg.Tunnel.Collection().Admins)
		return string(data), err
	case "tunnel.id":
		return primaryTunnelInstance(cfg).ID, nil
	case "tunnel.api_key":
		return primaryTunnelInstance(cfg).APIKey, nil
	case "tunnel.admin_key":
		return primaryTunnelAdmin(cfg).AdminKey, nil
	case "tunnel.admin_organization_id":
		return primaryTunnelAdmin(cfg).OrganizationID, nil
	case "tunnel.admin_workspace_id":
		return primaryTunnelAdmin(cfg).WorkspaceID, nil
	case "tunnel.admin_tenant_id":
		return primaryTunnelAdmin(cfg).TenantID, nil
	case "tunnel.control_plane_base_url":
		return firstNonEmpty(primaryTunnelInstance(cfg).ControlPlaneBaseURL, primaryTunnelAdmin(cfg).ControlPlaneBaseURL), nil
	case "tunnel.organization_id":
		return primaryTunnelInstance(cfg).OrganizationID, nil
	case "notifications.enabled":
		return strconv.FormatBool(cfg.Notifications.Enabled), nil
	case "notifications.approvals":
		return strconv.FormatBool(cfg.Notifications.Approvals), nil
	case "notifications.when_tui_inactive":
		return strconv.FormatBool(cfg.Notifications.WhenTUIInactive), nil
	case "notifications.open_action":
		return cfg.Notifications.OpenAction, nil
	default:
		return "", fmt.Errorf("unsupported config key: %s", key)
	}
}

func DisplayValue(cfg Config, spec FieldSpec) (string, error) {
	value, err := RawValue(cfg, spec.Key)
	if err != nil {
		return "", err
	}
	if spec.Sensitive {
		if strings.TrimSpace(value) == "" {
			return "not configured", nil
		}
		return "configured", nil
	}
	if spec.Kind == FieldList && strings.TrimSpace(value) == "" {
		return "none", nil
	}
	if strings.TrimSpace(value) == "" {
		return "-", nil
	}
	return value, nil
}

func State(cfg Config, spec FieldSpec) (FieldState, error) {
	if !spec.Editable || spec.Kind == FieldReadOnly {
		return FieldStateManaged, nil
	}
	current, err := comparableFieldValue(cfg, spec)
	if err != nil {
		return "", err
	}
	defaults := Default()
	baseline, err := comparableFieldValue(defaults, spec)
	if err != nil {
		return "", err
	}
	if current == baseline {
		return FieldStateDefault, nil
	}
	return FieldStateCustom, nil
}

func comparableFieldValue(cfg Config, spec FieldSpec) (string, error) {
	value, err := RawValue(cfg, spec.Key)
	if err != nil {
		return "", err
	}
	if spec.Sensitive {
		return "", nil
	}
	if spec.Kind != FieldList {
		return strings.TrimSpace(value), nil
	}
	items := splitFieldList(value)
	slices.Sort(items)
	return strings.Join(items, "\x00"), nil
}

func RedactedTree(cfg Config) (map[string]any, error) {
	data, err := configformat.Marshal(configformat.JSON, cfg)
	if err != nil {
		return nil, err
	}
	value, err := configformat.DecodeGeneric(configformat.JSON, data)
	if err != nil {
		return nil, err
	}
	tree, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("config view is not an object")
	}
	setTreeValue(tree, "auth.mcp_token_hash", RedactedValue)
	setTreeValue(tree, "auth.admin_token_hash", RedactedValue)
	setTreeValue(tree, "tunnel.api_key", RedactedValue)
	setTreeValue(tree, "tunnel.admin_key", RedactedValue)
	return tree, nil
}

func RedactedValueAt(cfg Config, key string) (any, error) {
	key = canonicalFieldKey(key)
	tree, err := RedactedTree(cfg)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return tree, nil
	}
	var current any = tree
	for _, part := range strings.Split(key, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("config key has no children: %s", key)
		}
		next, exists := object[part]
		if !exists {
			return nil, fmt.Errorf("unsupported config key: %s", key)
		}
		current = next
	}
	return current, nil
}

func canonicalFieldKey(key string) string {
	return strings.TrimSpace(key)
}

func setTreeValue(tree map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	current := tree
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
}

func splitFieldList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\n", ",")
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func parseBoolField(raw, key string) (bool, error) {
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func parseIntField(raw, key string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return value, nil
}

func primaryTunnelInstance(cfg Config) tunnel.InstanceConfig {
	if instances := cfg.RuntimeTunnels().Instances; len(instances) > 0 {
		return instances[0]
	}
	return tunnel.InstanceConfig{Enabled: cfg.Tunnel.Enabled, ID: cfg.Tunnel.ID, APIKey: cfg.Tunnel.APIKey, ControlPlaneBaseURL: cfg.Tunnel.ControlPlaneBaseURL, OrganizationID: cfg.Tunnel.OrganizationID}
}

func primaryTunnelAdmin(cfg Config) tunnel.AdminConfig {
	if admins := cfg.RuntimeTunnels().Admins; len(admins) > 0 {
		return admins[0]
	}
	return tunnel.AdminConfig{AdminKey: cfg.Tunnel.AdminKey, OrganizationID: cfg.Tunnel.AdminOrganizationID, WorkspaceID: cfg.Tunnel.AdminWorkspaceID, TenantID: cfg.Tunnel.AdminTenantID, ControlPlaneBaseURL: cfg.Tunnel.ControlPlaneBaseURL}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
