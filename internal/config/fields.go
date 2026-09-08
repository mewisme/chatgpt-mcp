package config

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
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
	FieldSectionRuntime  FieldSection = "runtime"
	FieldSectionAccess   FieldSection = "access"
	FieldSectionShell    FieldSection = "shell"
	FieldSectionFeatures FieldSection = "features"
	FieldSectionTunnel   FieldSection = "tunnel"
)

type FieldState string

const (
	FieldStateDefault FieldState = "default"
	FieldStateCustom  FieldState = "custom"
	FieldStateManaged FieldState = "managed"
)

var fieldSpecs = []FieldSpec{
	{Key: "server.enabled", Label: "MCP HTTP server", Section: FieldSectionRuntime, Description: "MCP HTTP transport enabled", Kind: FieldBool, Editable: true},
	{Key: "server.expose.mode", Label: "Exposure", Section: FieldSectionRuntime, Description: "network exposure mode", Kind: FieldEnum, Options: []string{"none", "all", "0.0.0.0", "interfaces"}, Editable: true},
	{Key: "server.expose.interfaces", Label: "Exposure interfaces", Section: FieldSectionRuntime, Description: "network interfaces used by interfaces exposure mode", Kind: FieldList, Editable: true},
	{Key: "server.port", Label: "MCP HTTP port", Section: FieldSectionRuntime, Description: "MCP HTTP server port", Kind: FieldInt, Editable: true},
	{Key: "server.allow_insecure_http", Label: "Allow insecure HTTP", Section: FieldSectionRuntime, Description: "allow authenticated HTTP beyond loopback", Kind: FieldBool, Editable: true},
	{Key: "admin.enabled", Label: "Admin server", Section: FieldSectionRuntime, Description: "admin server enabled", Kind: FieldBool, Editable: true},
	{Key: "admin.port", Label: "Admin port", Section: FieldSectionRuntime, Description: "admin server port", Kind: FieldInt, Editable: true},
	{Key: "auth.mcp_enabled", Label: "MCP authentication", Section: FieldSectionAccess, Description: "MCP authentication enabled", Kind: FieldBool, Editable: true},
	{Key: "auth.admin_enabled", Label: "Admin authentication", Section: FieldSectionAccess, Description: "admin authentication enabled", Kind: FieldBool, Editable: true},
	{Key: "auth.mcp_token_hash", Label: "MCP credential", Section: FieldSectionAccess, Description: "MCP token credential", Kind: FieldReadOnly, Sensitive: true, Guidance: "Manage this credential with the MCP auth token workflow."},
	{Key: "auth.admin_token_hash", Label: "Admin credential", Section: FieldSectionAccess, Description: "admin token credential", Kind: FieldReadOnly, Sensitive: true, Guidance: "Manage this credential with the admin auth token workflow."},
	{Key: "permissions.allow_dirs", Label: "Allowed directories", Section: FieldSectionAccess, Description: "additional filesystem roots", Kind: FieldList, Editable: true},
	{Key: "shell.path", Label: "Executable search paths", Section: FieldSectionShell, Description: "additional executable search paths", Kind: FieldList, Editable: true},
	{Key: "shell.approval_policy", Label: "Approval policy", Section: FieldSectionShell, Description: "controls when shell commands require local approval", Details: "Explicit deny rules take precedence over allow rules. The selected policy then determines whether commands that are not covered by an explicit rule require local approval.", Kind: FieldEnum, Options: []string{"allow", "balanced", "strict", "deny"}, Values: []FieldValueSpec{{Value: "allow", Description: "Run commands without approval unless an explicit deny rule matches."}, {Value: "balanced", Description: "Require approval for guarded or risky operations and external access."}, {Value: "strict", Description: "Require approval for execution that is not recognized as a workspace-confined static read."}, {Value: "deny", Description: "Require approval for commands unless an explicit allow rule matches."}}, Editable: true, Related: []string{"shell.approval_allow_commands", "shell.approval_deny_commands", "shell.network_policy"}},
	{Key: "shell.approval_allow_commands", Label: "Allow commands", Section: FieldSectionShell, Description: "argv-aware glob patterns that bypass ordinary approval gates when every invocation matches; supports *, ?, [], and standalone **", Kind: FieldList, Editable: true},
	{Key: "shell.approval_deny_commands", Label: "Deny commands", Section: FieldSectionShell, Description: "argv-aware glob patterns that always require approval; deny rules take precedence over allow rules", Kind: FieldList, Editable: true},
	{Key: "shell.environment_policy", Label: "Environment policy", Section: FieldSectionShell, Description: "shell environment inheritance policy", Kind: FieldEnum, Options: []string{"auto", "inherit", "filtered", "minimal"}, Editable: true},
	{Key: "shell.environment_allow", Label: "Environment allow", Section: FieldSectionShell, Description: "environment variables explicitly exposed to shell commands", Kind: FieldList, Editable: true},
	{Key: "shell.sandbox_policy", Label: "Sandbox policy", Section: FieldSectionShell, Description: "OS-level shell sandbox policy", Kind: FieldEnum, Options: []string{"auto", "off", "required"}, Editable: true},
	{Key: "shell.network_policy", Label: "Network policy", Section: FieldSectionShell, Description: "shell network egress policy", Kind: FieldEnum, Options: []string{"auto", "inherit", "deny"}, Editable: true},
	{Key: "features.ponytail.active", Label: "Ponytail active", Section: FieldSectionFeatures, Description: "Ponytail mode active by default", Kind: FieldBool, Editable: true},
	{Key: "features.ponytail.mode", Label: "Ponytail mode", Section: FieldSectionFeatures, Description: "Ponytail default intensity", Kind: FieldEnum, Options: []string{"lite", "full", "ultra"}, Editable: true},
	{Key: "features.caveman.active", Label: "Caveman active", Section: FieldSectionFeatures, Description: "Caveman mode active by default", Kind: FieldBool, Editable: true},
	{Key: "features.caveman.mode", Label: "Caveman mode", Section: FieldSectionFeatures, Description: "Caveman default intensity", Kind: FieldEnum, Options: []string{"lite", "full", "ultra", "wenyan-lite", "wenyan-full", "wenyan-ultra"}, Editable: true},
	{Key: "tunnel.enabled", Label: "Tunnel", Section: FieldSectionTunnel, Description: "OpenAI Secure MCP Tunnel enabled", Kind: FieldBool, Editable: true},
	{Key: "tunnel.id", Label: "Tunnel ID", Section: FieldSectionTunnel, Description: "OpenAI tunnel ID", Kind: FieldString, Editable: true},
	{Key: "tunnel.api_key", Label: "Runtime API key", Section: FieldSectionTunnel, Description: "OpenAI tunnel runtime API key", Kind: FieldReadOnly, Sensitive: true, Guidance: "Manage the runtime key from the Tunnel page."},
	{Key: "tunnel.admin_key", Label: "Admin key", Section: FieldSectionTunnel, Description: "OpenAI tunnel admin key", Kind: FieldReadOnly, Sensitive: true, Guidance: "Manage and verify the admin key from the Tunnel page."},
	{Key: "tunnel.admin_organization_id", Label: "Admin organization scope", Section: FieldSectionTunnel, Description: "verified tunnel admin organization scope", Kind: FieldReadOnly, Guidance: "This scope is set only after admin-key verification."},
	{Key: "tunnel.admin_workspace_id", Label: "Admin workspace scope", Section: FieldSectionTunnel, Description: "verified tunnel admin workspace scope", Kind: FieldReadOnly, Guidance: "This scope is set only after admin-key verification."},
	{Key: "tunnel.admin_tenant_id", Label: "Admin tenant scope", Section: FieldSectionTunnel, Description: "verified tunnel admin tenant scope", Kind: FieldReadOnly, Guidance: "This scope is set only after admin-key verification."},
	{Key: "tunnel.control_plane_base_url", Label: "Control-plane URL", Section: FieldSectionTunnel, Description: "OpenAI tunnel control-plane URL", Kind: FieldString, Editable: true},
	{Key: "tunnel.organization_id", Label: "Organization ID", Section: FieldSectionTunnel, Description: "OpenAI organization ID", Kind: FieldString, Editable: true},
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
	case "auth.admin_enabled":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Auth.AdminEnabled = value
	case "permissions.allow_dirs":
		cfg.Permissions.AllowDirs = splitFieldList(raw)
	case "shell.path":
		cfg.Shell.Path = splitFieldList(raw)
	case "shell.approval_policy":
		value, err := NormalizeShellApprovalPolicy(raw)
		if err != nil {
			return err
		}
		cfg.Shell.ApprovalPolicy = value
	case "shell.approval_allow_commands":
		value, err := NormalizeShellApprovalCommands(splitFieldList(raw))
		if err != nil {
			return err
		}
		cfg.Shell.ApprovalAllowCommands = value
	case "shell.approval_deny_commands":
		value, err := NormalizeShellApprovalCommands(splitFieldList(raw))
		if err != nil {
			return err
		}
		cfg.Shell.ApprovalDenyCommands = value
	case "shell.environment_policy":
		value, err := NormalizeShellEnvironmentPolicy(raw)
		if err != nil {
			return err
		}
		cfg.Shell.EnvironmentPolicy = value
	case "shell.environment_allow":
		value, err := NormalizeShellEnvironmentAllow(splitFieldList(raw))
		if err != nil {
			return err
		}
		cfg.Shell.EnvironmentAllow = value
	case "shell.sandbox_policy":
		value, err := NormalizeShellSandboxPolicy(raw)
		if err != nil {
			return err
		}
		cfg.Shell.SandboxPolicy = value
	case "shell.network_policy":
		value, err := NormalizeShellNetworkPolicy(raw)
		if err != nil {
			return err
		}
		cfg.Shell.NetworkPolicy = value
	case "features.ponytail.active":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Features.Ponytail.Active = value
	case "features.ponytail.mode":
		value := strings.ToLower(strings.TrimSpace(raw))
		if value != "lite" && value != "full" && value != "ultra" {
			return errors.New("features.ponytail.mode must be lite, full, or ultra")
		}
		cfg.Features.Ponytail.Mode = value
	case "features.caveman.active":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Features.Caveman.Active = value
	case "features.caveman.mode":
		value := strings.ToLower(strings.TrimSpace(raw))
		switch value {
		case "lite", "full", "ultra", "wenyan-lite", "wenyan-full", "wenyan-ultra":
			cfg.Features.Caveman.Mode = value
		default:
			return errors.New("features.caveman.mode must be lite, full, ultra, wenyan-lite, wenyan-full, or wenyan-ultra")
		}
	case "tunnel.enabled":
		value, err := parseBoolField(raw, key)
		if err != nil {
			return err
		}
		cfg.Tunnel.Enabled = value
	case "tunnel.id":
		cfg.Tunnel.ID = raw
	case "tunnel.api_key":
		cfg.Tunnel.APIKey = raw
	case "tunnel.admin_key", "tunnel.admin_organization_id", "tunnel.admin_workspace_id", "tunnel.admin_tenant_id":
		return errors.New("tunnel admin credentials cannot be set through config; use chatgpt-mcp tunnel admin key")
	case "tunnel.control_plane_base_url":
		cfg.Tunnel.ControlPlaneBaseURL = raw
	case "tunnel.organization_id":
		cfg.Tunnel.OrganizationID = raw
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
	case "admin.enabled":
		return strconv.FormatBool(cfg.Admin.Enabled), nil
	case "admin.port":
		return strconv.Itoa(cfg.Admin.Port), nil
	case "auth.mcp_enabled":
		return strconv.FormatBool(cfg.Auth.MCPEnabled), nil
	case "auth.admin_enabled":
		return strconv.FormatBool(cfg.Auth.AdminEnabled), nil
	case "auth.mcp_token_hash":
		return cfg.Auth.MCPTokenHash, nil
	case "auth.admin_token_hash":
		return cfg.Auth.AdminTokenHash, nil
	case "permissions.allow_dirs":
		return strings.Join(cfg.Permissions.AllowDirs, ","), nil
	case "shell.path":
		return strings.Join(cfg.Shell.Path, ","), nil
	case "shell.approval_policy":
		return cfg.Shell.ApprovalPolicy, nil
	case "shell.approval_allow_commands":
		return strings.Join(cfg.Shell.ApprovalAllowCommands, ","), nil
	case "shell.approval_deny_commands":
		return strings.Join(cfg.Shell.ApprovalDenyCommands, ","), nil
	case "shell.environment_policy":
		return cfg.Shell.EnvironmentPolicy, nil
	case "shell.environment_allow":
		return strings.Join(cfg.Shell.EnvironmentAllow, ","), nil
	case "shell.sandbox_policy":
		return cfg.Shell.SandboxPolicy, nil
	case "shell.network_policy":
		return cfg.Shell.NetworkPolicy, nil
	case "features.ponytail.active":
		return strconv.FormatBool(cfg.Features.Ponytail.Active), nil
	case "features.ponytail.mode":
		return cfg.Features.Ponytail.Mode, nil
	case "features.caveman.active":
		return strconv.FormatBool(cfg.Features.Caveman.Active), nil
	case "features.caveman.mode":
		return cfg.Features.Caveman.Mode, nil
	case "tunnel.enabled":
		return strconv.FormatBool(cfg.Tunnel.Enabled), nil
	case "tunnel.id":
		return cfg.Tunnel.ID, nil
	case "tunnel.api_key":
		return cfg.Tunnel.APIKey, nil
	case "tunnel.admin_key":
		return cfg.Tunnel.AdminKey, nil
	case "tunnel.admin_organization_id":
		return cfg.Tunnel.AdminOrganizationID, nil
	case "tunnel.admin_workspace_id":
		return cfg.Tunnel.AdminWorkspaceID, nil
	case "tunnel.admin_tenant_id":
		return cfg.Tunnel.AdminTenantID, nil
	case "tunnel.control_plane_base_url":
		return cfg.Tunnel.ControlPlaneBaseURL, nil
	case "tunnel.organization_id":
		return cfg.Tunnel.OrganizationID, nil
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
	key = strings.TrimSpace(key)
	switch key {
	case "features.ponytail.enabled":
		return "features.ponytail.active"
	case "features.caveman.enabled":
		return "features.caveman.active"
	default:
		return key
	}
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
