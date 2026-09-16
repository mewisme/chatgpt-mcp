package page

import (
	"fmt"
	"strings"

	"charm.land/huh/v2"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type tunnelRuntimeFormData struct {
	Enabled        bool
	ID             string
	RuntimeAPIKey  string
	ControlPlane   string
	OrganizationID string
}

type tunnelAdminProfileFormData struct {
	ID           string
	AdminKey     string
	ScopeKind    string
	ScopeID      string
	ControlPlane string
}

func newTunnelAdminProfileEditor(profile application.TunnelAdminProfile, create bool) (component.Editor, *tunnelAdminProfileFormData) {
	data := &tunnelAdminProfileFormData{ID: profile.ID, ScopeKind: "organization", ControlPlane: profile.ControlPlaneBaseURL}
	switch {
	case profile.OrganizationID != "":
		data.ScopeKind, data.ScopeID = "organization", profile.OrganizationID
	case profile.WorkspaceID != "":
		data.ScopeKind, data.ScopeID = "workspace", profile.WorkspaceID
	case profile.TenantID != "":
		data.ScopeKind, data.ScopeID = "tenant", profile.TenantID
	}
	fields := []huh.Field{}
	if create {
		fields = append(fields, component.Input("Profile ID", &data.ID).Placeholder("personal").Validate(requiredValue("profile id")))
	}
	key := component.PasswordInput("OpenAI admin API key", &data.AdminKey)
	if create {
		key = key.Validate(requiredValue("admin API key"))
	} else {
		key = key.Placeholder("Blank keeps the current key.")
	}
	fields = append(fields,
		key,
		component.Select("Scope type", &data.ScopeKind,
			huh.NewOption("Organization", "organization"),
			huh.NewOption("Workspace", "workspace"),
			huh.NewOption("Tenant", "tenant"),
		),
		component.Input("Scope ID", &data.ScopeID).Validate(requiredValue("scope id")),
		component.Input("Control plane base URL", &data.ControlPlane).Placeholder("Blank uses the default OpenAI endpoint."),
	)
	primary := "save"
	if create {
		primary = "add"
	}
	title, description := "Admin Profile", "Update this management credential. Changes are verified before they are saved."
	if create {
		title, description = "Admin Profile", "Add a named OpenAI admin credential. Verification must succeed before the profile is saved."
	}
	editor := component.NewEditor(primary, component.EditorSection{ID: "profile", Title: title, Description: description, Form: component.NewEditorForm(component.Group(fields...))})
	return editor, data
}

func adminProfileFromForm(data *tunnelAdminProfileFormData, id string) (tunnel.AdminConfig, error) {
	if data == nil {
		return tunnel.AdminConfig{}, fmt.Errorf("admin profile draft is unavailable")
	}
	admin := tunnel.AdminConfig{ID: strings.TrimSpace(id), AdminKey: strings.TrimSpace(data.AdminKey), ControlPlaneBaseURL: strings.TrimSpace(data.ControlPlane)}
	if admin.ID == "" {
		admin.ID = strings.TrimSpace(data.ID)
	}
	switch data.ScopeKind {
	case "organization":
		admin.OrganizationID = strings.TrimSpace(data.ScopeID)
	case "workspace":
		admin.WorkspaceID = strings.TrimSpace(data.ScopeID)
	case "tenant":
		admin.TenantID = strings.TrimSpace(data.ScopeID)
	default:
		return tunnel.AdminConfig{}, fmt.Errorf("unsupported admin scope type %q", data.ScopeKind)
	}
	if err := tunnel.ValidateAdminScope(tunnel.AdminScope{OrganizationID: admin.OrganizationID, WorkspaceID: admin.WorkspaceID, TenantID: admin.TenantID}); err != nil {
		return tunnel.AdminConfig{}, err
	}
	return admin, nil
}

type tunnelAdminFormData struct {
	AdminKey  string
	ScopeKind string
	ScopeID   string
}

type managedTunnelFormData struct {
	AdminProfileID  string
	Name            string
	Description     string
	OrganizationIDs string
	WorkspaceIDs    string
	TenantIDs       string
}

type managedConfigureFormData struct {
	AdminProfileID string
	RuntimeKeyMode string
	RuntimeAPIKey  string
	ProjectID      string
}

type managedDeleteFormData struct{ AdminProfileID string }

func newTunnelRuntimeEditor(dashboard application.TunnelDashboard) (component.Editor, *tunnelRuntimeFormData) {
	data := &tunnelRuntimeFormData{Enabled: dashboard.Config.Enabled, ID: dashboard.Config.ID, ControlPlane: dashboard.Config.ControlPlaneBaseURL, OrganizationID: dashboard.Config.OrganizationID}
	enabledTitle := "Enabled (at least one MCP transport must remain enabled)"
	if !dashboard.MCPHTTPEnabled {
		enabledTitle = "Enabled (required while MCP HTTP is disabled)"
	}
	enabled := component.Switch(enabledTitle, &data.Enabled, "ENABLED", "DISABLED")
	if !dashboard.MCPHTTPEnabled {
		enabled.Validate(func(value bool) error {
			if !value {
				return fmt.Errorf("tunnel must remain enabled while MCP HTTP is disabled")
			}
			return nil
		})
	}
	editor := component.NewEditor("save", component.EditorSection{
		ID: "runtime", Title: "Runtime", Description: "Configure the selected runtime tunnel. Blank runtime API key keeps the current secret.",
		Form: component.NewEditorForm(component.Group(
			enabled,
			component.Input("Tunnel ID", &data.ID),
			component.PasswordInput("Runtime API key", &data.RuntimeAPIKey).Placeholder("Blank keeps the current key."),
			component.Input("Control plane base URL", &data.ControlPlane),
			component.Input("Organization ID", &data.OrganizationID),
		)),
	})
	return editor, data
}
func newTunnelAdminEditor(status application.TunnelAdminStatus) (component.Editor, *tunnelAdminFormData) {
	data := &tunnelAdminFormData{ScopeKind: "auto"}
	scope := status.Scope
	switch {
	case scope.OrganizationID != "":
		data.ScopeKind, data.ScopeID = "organization", scope.OrganizationID
	case scope.WorkspaceID != "":
		data.ScopeKind, data.ScopeID = "workspace", scope.WorkspaceID
	case scope.TenantID != "":
		data.ScopeKind, data.ScopeID = "tenant", scope.TenantID
	}
	editor := component.NewEditor("verify", component.EditorSection{
		ID: "admin-key", Title: "Admin Key", Description: "Store and verify an OpenAI admin key with Tunnels Manage access.",
		Form: component.NewEditorForm(component.Group(
			component.PasswordInput("OpenAI admin API key (Tunnels Manage)", &data.AdminKey),
			component.Select("Verification scope", &data.ScopeKind,
				huh.NewOption("Auto (reuse or derive)", "auto"), huh.NewOption("Organization", "organization"), huh.NewOption("Workspace", "workspace"), huh.NewOption("Tenant", "tenant"),
			),
			component.Input("Scope ID (ignored for Auto)", &data.ScopeID),
		)),
	})
	return editor, data
}
func newManagedTunnelEditor(metadata tunnel.Metadata, create bool, profiles []application.TunnelAdminProfile) (component.Editor, *managedTunnelFormData) {
	data := &managedTunnelFormData{
		Name: metadata.Name, Description: metadata.Description,
		OrganizationIDs: strings.Join(metadata.OrganizationIDs, "\n"), WorkspaceIDs: strings.Join(metadata.WorkspaceIDs, "\n"), TenantIDs: strings.Join(metadata.TenantIDs, "\n"),
	}
	profile := managedProfileSelect("Admin profile", &data.AdminProfileID, profiles, true)
	name := component.Input("Name", &data.Name).Validate(requiredValue("tunnel name"))
	description := component.Text("Description", &data.Description).Validate(requiredValue("tunnel description"))
	if !create {
		description = component.Text("Description", &data.Description)
	}
	primary := "save"
	if create {
		primary = "create"
	}
	editor := component.NewEditor(primary,
		component.EditorSection{ID: "admin", Title: "Admin", Description: "Choose the admin profile used for this management operation.", Form: component.NewEditorForm(component.Group(profile))},
		component.EditorSection{ID: "general", Title: "General", Description: "Name and description for the managed tunnel.", Form: component.NewEditorForm(component.Group(name, description))},
		component.EditorSection{ID: "scope", Title: "Scope", Description: "Optional organization, workspace, and tenant IDs. Enter one ID per line.", Form: component.NewEditorForm(component.Group(
			component.Text("Organization IDs (one per line)", &data.OrganizationIDs),
			component.Text("Workspace IDs (one per line)", &data.WorkspaceIDs),
			component.Text("Tenant IDs (one per line)", &data.TenantIDs),
		))},
	)
	return editor, data
}

func newManagedConfigureEditor(profiles []application.TunnelAdminProfile) (component.Editor, *managedConfigureFormData) {
	data := &managedConfigureFormData{RuntimeKeyMode: "auto"}
	editor := component.NewEditor("attach",
		component.EditorSection{ID: "admin", Title: "Admin", Description: "Choose the admin profile used to fetch and attach this tunnel.", Form: component.NewEditorForm(component.Group(managedProfileSelect("Admin profile", &data.AdminProfileID, profiles, false)))},
		component.EditorSection{
			ID: "runtime", Title: "Runtime", Description: "Attach this managed tunnel as another ingress into the shared local runtime.",
			Form: component.NewEditorForm(
				component.Group(component.Select("Runtime credential", &data.RuntimeKeyMode, huh.NewOption("Auto generate with admin key", "auto"), huh.NewOption("Enter runtime key manually", "manual"))),
				component.Group(component.Input("OpenAI project ID (optional)", &data.ProjectID).Placeholder("Blank uses the only active project or Default project.")).WithHideFunc(func() bool { return data.RuntimeKeyMode != "auto" }),
				component.Group(component.PasswordInput("Runtime API key", &data.RuntimeAPIKey).Placeholder("Read + Use key.").Validate(func(value string) error {
					if data.RuntimeKeyMode == "manual" && strings.TrimSpace(value) == "" {
						return fmt.Errorf("runtime API key is required")
					}
					return nil
				})).WithHideFunc(func() bool { return data.RuntimeKeyMode != "manual" }),
			),
		})
	return editor, data
}

func newManagedDeleteEditor(profiles []application.TunnelAdminProfile) (component.Editor, *managedDeleteFormData) {
	data := &managedDeleteFormData{}
	editor := component.NewEditor("delete", component.EditorSection{ID: "admin", Title: "Admin", Description: "Choose the admin profile used to permanently delete the remote tunnel.", Form: component.NewEditorForm(component.Group(managedProfileSelect("Admin profile", &data.AdminProfileID, profiles, true)))})
	return editor, data
}

func managedProfileSelect(title string, value *string, profiles []application.TunnelAdminProfile, manage bool) *huh.Select[string] {
	options := make([]huh.Option[string], 0, len(profiles)+1)
	eligible := 0
	for _, profile := range profiles {
		if manage && !profile.ManageAccess || !manage && !profile.ReadAccess && !profile.ManageAccess {
			continue
		}
		eligible++
		options = append(options, huh.NewOption(profile.ID, profile.ID))
	}
	if eligible == 1 {
		*value = options[0].Value
	} else {
		options = append([]huh.Option[string]{huh.NewOption("Select profile", "")}, options...)
	}
	return component.Select(title, value, options...).Validate(requiredValue("admin profile"))
}

func runtimeInputFromForm(data *tunnelRuntimeFormData) application.TunnelRuntimeInput {
	input := application.TunnelRuntimeInput{Enabled: &data.Enabled, ID: &data.ID, ControlPlaneBaseURL: &data.ControlPlane, OrganizationID: &data.OrganizationID}
	if strings.TrimSpace(data.RuntimeAPIKey) != "" {
		input.APIKey = &data.RuntimeAPIKey
	}
	return input
}

func adminInputFromForm(data *tunnelAdminFormData) application.TunnelAdminKeyInput {
	input := application.TunnelAdminKeyInput{Key: data.AdminKey}
	if data.ScopeKind == "auto" {
		return input
	}
	scope := tunnel.AdminScope{}
	switch data.ScopeKind {
	case "organization":
		scope.OrganizationID = data.ScopeID
	case "workspace":
		scope.WorkspaceID = data.ScopeID
	case "tenant":
		scope.TenantID = data.ScopeID
	}
	input.Scope = &scope
	return input
}

func managedCreateInput(data *managedTunnelFormData) tunnel.CreateRequest {
	request := tunnel.CreateRequest{
		Name: strings.TrimSpace(data.Name), Description: strings.TrimSpace(data.Description),
		OrganizationIDs: application.NormalizeTunnelIDs(splitLines(data.OrganizationIDs)), WorkspaceIDs: application.NormalizeTunnelIDs(splitLines(data.WorkspaceIDs)), TenantIDs: application.NormalizeTunnelIDs(splitLines(data.TenantIDs)),
	}
	return request
}

func managedUpdateInput(data *managedTunnelFormData) tunnel.UpdateRequest {
	name, description := strings.TrimSpace(data.Name), data.Description
	organizations, workspaces, tenants := application.NormalizeTunnelIDs(splitLines(data.OrganizationIDs)), application.NormalizeTunnelIDs(splitLines(data.WorkspaceIDs)), application.NormalizeTunnelIDs(splitLines(data.TenantIDs))
	request := tunnel.UpdateRequest{Name: &name, Description: &description, OrganizationIDs: &organizations, WorkspaceIDs: &workspaces, TenantIDs: &tenants}
	return request
}
