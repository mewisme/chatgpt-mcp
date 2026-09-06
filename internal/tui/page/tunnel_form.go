package page

import (
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

type tunnelAdminFormData struct {
	AdminKey  string
	ScopeKind string
	ScopeID   string
}

type managedTunnelFormData struct {
	Name            string
	Description     string
	OrganizationIDs string
	WorkspaceIDs    string
	TenantIDs       string
	Configure       bool
	RuntimeAPIKey   string
	Enable          bool
}

type managedConfigureFormData struct {
	RuntimeAPIKey string
	Enable        bool
}

func newTunnelRuntimeForm(dashboard application.TunnelDashboard) (component.Form, *tunnelRuntimeFormData) {
	data := &tunnelRuntimeFormData{Enabled: dashboard.Config.Enabled, ID: dashboard.Config.ID, ControlPlane: dashboard.Config.ControlPlaneBaseURL, OrganizationID: dashboard.Config.OrganizationID}
	form := component.NewForm(component.Group(
		component.Switch("Enabled", &data.Enabled),
		component.Input("Tunnel ID", &data.ID),
		component.PasswordInput("Runtime API key", &data.RuntimeAPIKey).Description("Blank keeps the current key."),
		component.Input("Control plane base URL", &data.ControlPlane),
		component.Input("Organization ID", &data.OrganizationID),
	))
	return form, data
}

func newTunnelAdminForm(status application.TunnelAdminStatus) (component.Form, *tunnelAdminFormData) {
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
	form := component.NewForm(component.Group(
		component.PasswordInput("Admin API key", &data.AdminKey).Description("OpenAI admin key with Tunnels Manage access."),
		component.Select("Verification scope", &data.ScopeKind,
			huh.NewOption("Auto (reuse or derive)", "auto"), huh.NewOption("Organization", "organization"), huh.NewOption("Workspace", "workspace"), huh.NewOption("Tenant", "tenant"),
		),
		component.Input("Scope ID", &data.ScopeID).Description("Ignored when scope is Auto."),
	))
	return form, data
}

func newManagedTunnelForm(metadata tunnel.Metadata, create bool) (component.Form, *managedTunnelFormData) {
	data := &managedTunnelFormData{
		Name: metadata.Name, Description: metadata.Description,
		OrganizationIDs: strings.Join(metadata.OrganizationIDs, "\n"), WorkspaceIDs: strings.Join(metadata.WorkspaceIDs, "\n"), TenantIDs: strings.Join(metadata.TenantIDs, "\n"),
	}
	name := component.Input("Name", &data.Name).Validate(requiredValue("tunnel name"))
	description := component.Text("Description", &data.Description).Validate(requiredValue("tunnel description"))
	if !create {
		description = component.Text("Description", &data.Description)
	}
	form := component.NewForm(
		component.Group(name, description),
		component.Group(
			component.Text("Organization IDs (one per line)", &data.OrganizationIDs),
			component.Text("Workspace IDs (one per line)", &data.WorkspaceIDs),
			component.Text("Tenant IDs (one per line)", &data.TenantIDs),
		),
		component.Group(
			component.Confirm("Configure cgm to use this tunnel", &data.Configure),
			component.PasswordInput("Runtime API key", &data.RuntimeAPIKey).Description("Blank reuses the current runtime key."),
			component.Confirm("Enable tunnel after configure", &data.Enable),
		),
	)
	return form, data
}

func newManagedConfigureForm() (component.Form, *managedConfigureFormData) {
	data := &managedConfigureFormData{}
	form := component.NewForm(component.Group(
		component.PasswordInput("Runtime API key", &data.RuntimeAPIKey).Description("Blank reuses the current runtime key."),
		component.Confirm("Enable tunnel", &data.Enable),
	))
	return form, data
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

func managedCreateInput(data *managedTunnelFormData) (tunnel.CreateRequest, application.ManagedTunnelOptions) {
	request := tunnel.CreateRequest{
		Name: strings.TrimSpace(data.Name), Description: strings.TrimSpace(data.Description),
		OrganizationIDs: application.NormalizeTunnelIDs(splitLines(data.OrganizationIDs)), WorkspaceIDs: application.NormalizeTunnelIDs(splitLines(data.WorkspaceIDs)), TenantIDs: application.NormalizeTunnelIDs(splitLines(data.TenantIDs)),
	}
	return request, application.ManagedTunnelOptions{Configure: data.Configure, RuntimeAPIKey: data.RuntimeAPIKey, Enable: data.Enable}
}

func managedUpdateInput(data *managedTunnelFormData) (tunnel.UpdateRequest, application.ManagedTunnelOptions) {
	name, description := strings.TrimSpace(data.Name), data.Description
	organizations, workspaces, tenants := application.NormalizeTunnelIDs(splitLines(data.OrganizationIDs)), application.NormalizeTunnelIDs(splitLines(data.WorkspaceIDs)), application.NormalizeTunnelIDs(splitLines(data.TenantIDs))
	request := tunnel.UpdateRequest{Name: &name, Description: &description, OrganizationIDs: &organizations, WorkspaceIDs: &workspaces, TenantIDs: &tenants}
	return request, application.ManagedTunnelOptions{Configure: data.Configure, RuntimeAPIKey: data.RuntimeAPIKey, Enable: data.Enable}
}
