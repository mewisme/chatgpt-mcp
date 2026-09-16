package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/pluginhost"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type localTunnelRequest struct {
	ID                  string `json:"id"`
	Enabled             bool   `json:"enabled"`
	APIKey              string `json:"api_key,omitempty"`
	AdminProfileID      string `json:"admin_profile_id,omitempty"`
	ControlPlaneBaseURL string `json:"control_plane_base_url,omitempty"`
	OrganizationID      string `json:"organization_id,omitempty"`
}

type localTunnelView struct {
	ID                   string        `json:"id"`
	Enabled              bool          `json:"enabled"`
	RuntimeKeyConfigured bool          `json:"runtime_key_configured"`
	AdminProfileID       string        `json:"admin_profile_id,omitempty"`
	ControlPlaneBaseURL  string        `json:"control_plane_base_url,omitempty"`
	OrganizationID       string        `json:"organization_id,omitempty"`
	Status               tunnel.Status `json:"status"`
}

type managedTunnelCreateRequest struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	TenantIDs       []string `json:"tenant_ids,omitempty"`
	WorkspaceIDs    []string `json:"workspace_ids,omitempty"`
	OrganizationIDs []string `json:"organization_ids,omitempty"`
}

type managedTunnelUpdateRequest struct {
	Name            *string   `json:"name,omitempty"`
	Description     *string   `json:"description,omitempty"`
	TenantIDs       *[]string `json:"tenant_ids,omitempty"`
	WorkspaceIDs    *[]string `json:"workspace_ids,omitempty"`
	OrganizationIDs *[]string `json:"organization_ids,omitempty"`
}

func localView(instance tunnel.InstanceConfig, status tunnel.Status) localTunnelView {
	status.AdminKeyConfigured = false
	status.AdminScope = nil
	return localTunnelView{ID: instance.ID, Enabled: instance.Enabled, RuntimeKeyConfigured: instance.APIKey != "", AdminProfileID: instance.AdminProfileID, ControlPlaneBaseURL: instance.ControlPlaneBaseURL, OrganizationID: instance.OrganizationID, Status: status}
}

func (api API) collectionState() (config.Config, tunnel.CollectionConfig, error) {
	if api.Config == nil {
		return config.Config{}, tunnel.CollectionConfig{}, errors.New("tunnel runtime unavailable")
	}
	cfg := api.Config.Snapshot()
	return cfg, cfg.RuntimeTunnels(), nil
}

func (api API) syncLiveConfig(next config.Config) error {
	if api.ReloadConfig != nil {
		return api.ReloadConfig(next)
	}
	if api.Config != nil {
		_, err := api.Config.Update(func(config.Config) (config.Config, error) { return next, nil })
		return err
	}
	return nil
}

func (api API) reloadCollectionFromDisk() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return api.syncLiveConfig(cfg)
}

func writeTunnelCollectionError(w http.ResponseWriter, err error) {
	msg := err.Error()
	code := http.StatusBadRequest
	switch {
	case strings.Contains(msg, "not attached"), strings.Contains(msg, "not found"):
		code = http.StatusNotFound
	case strings.Contains(msg, "already attached"), strings.Contains(msg, "already exists"), strings.Contains(msg, "is used by"):
		code = http.StatusConflict
	}
	http.Error(w, msg, code)
}

func (api API) liveLocalView(id string) (localTunnelView, error) {
	_, collection, err := api.collectionState()
	if err != nil {
		return localTunnelView{}, err
	}
	for _, instance := range collection.Instances {
		if instance.ID != id {
			continue
		}
		return localView(instance, api.instanceStatus(instance)), nil
	}
	return localTunnelView{}, fmt.Errorf("tunnel %q is not attached", id)
}

func (api API) instanceStatus(instance tunnel.InstanceConfig) tunnel.Status {
	status := tunnel.Status{ID: instance.ID, Enabled: instance.Enabled, Provider: tunnel.ProviderOpenAI}
	statuses, err := application.SecureMCPRuntimeStatuses(context.Background(), pluginhost.RuntimeHost)
	if err != nil {
		return status
	}
	for _, item := range statuses {
		if item.ID == instance.ID {
			return item
		}
	}
	return status
}

func (api API) handleLocalTunnels(w http.ResponseWriter, r *http.Request) {
	_, collection, err := api.collectionState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		views := make([]localTunnelView, 0, len(collection.Instances))
		for _, instance := range collection.Instances {
			views = append(views, localView(instance, api.instanceStatus(instance)))
		}
		writeJSON(w, views)
	case http.MethodPost:
		var request localTunnelRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item, err := application.AttachLocalTunnel(r.Context(), tunnel.InstanceConfig{ID: strings.TrimSpace(request.ID), Enabled: request.Enabled, APIKey: strings.TrimSpace(request.APIKey), AdminProfileID: strings.TrimSpace(request.AdminProfileID), ControlPlaneBaseURL: strings.TrimSpace(request.ControlPlaneBaseURL), OrganizationID: strings.TrimSpace(request.OrganizationID)})
		if err != nil {
			writeTunnelCollectionError(w, err)
			return
		}
		if err := api.reloadCollectionFromDisk(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		view, err := api.liveLocalView(item.ID)
		if err != nil {
			writeJSON(w, localView(tunnel.InstanceConfig{ID: item.ID, Enabled: item.Enabled, APIKey: "", AdminProfileID: item.AdminProfileID, ControlPlaneBaseURL: item.ControlPlaneBaseURL, OrganizationID: item.OrganizationID}, item.Status))
			return
		}
		writeJSON(w, view)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) handleLocalTunnel(w http.ResponseWriter, r *http.Request) {
	cfg, collection, err := api.collectionState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tunnels/"), "/")
	if path == "" {
		http.Error(w, "tunnel id is required", http.StatusBadRequest)
		return
	}
	parts := strings.Split(path, "/")
	id := parts[0]
	index := -1
	for i, instance := range collection.Instances {
		if instance.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		http.Error(w, "tunnel is not attached", http.StatusNotFound)
		return
	}
	if len(parts) == 2 {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch parts[1] {
		case "start":
			_, err = application.StartSecureMCPInstance(r.Context(), pluginhost.RuntimeHost, id)
		case "stop":
			_, err = application.StopSecureMCPInstance(r.Context(), pluginhost.RuntimeHost, cfg.Server.Enabled, id)
		case "enable", "disable":
			_, err = application.SetLocalTunnelEnabled(r.Context(), id, parts[1] == "enable")
			if err != nil {
				writeTunnelCollectionError(w, err)
				return
			}
			if err := api.reloadCollectionFromDisk(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, "unknown tunnel action", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		view, viewErr := api.liveLocalView(id)
		if viewErr != nil {
			http.Error(w, viewErr.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, view)
		return
	}
	if len(parts) != 1 {
		http.Error(w, "unknown tunnel path", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, localView(collection.Instances[index], api.instanceStatus(collection.Instances[index])))
	case http.MethodPut:
		var request localTunnelRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if request.ID != "" && request.ID != id {
			http.Error(w, "tunnel id cannot be changed", http.StatusBadRequest)
			return
		}
		instance := tunnel.InstanceConfig{ID: id, Enabled: request.Enabled, APIKey: strings.TrimSpace(request.APIKey), AdminProfileID: strings.TrimSpace(request.AdminProfileID), ControlPlaneBaseURL: strings.TrimSpace(request.ControlPlaneBaseURL), OrganizationID: strings.TrimSpace(request.OrganizationID)}
		if _, err := application.UpdateLocalTunnel(r.Context(), instance); err != nil {
			writeTunnelCollectionError(w, err)
			return
		}
		if err := api.reloadCollectionFromDisk(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		view, err := api.liveLocalView(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, view)
	case http.MethodDelete:
		if err := application.DetachLocalTunnel(r.Context(), id); err != nil {
			writeTunnelCollectionError(w, err)
			return
		}
		if err := api.reloadCollectionFromDisk(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type adminProfileRequest struct {
	ID                  string `json:"id"`
	AdminKey            string `json:"admin_key,omitempty"`
	OrganizationID      string `json:"organization_id,omitempty"`
	WorkspaceID         string `json:"workspace_id,omitempty"`
	TenantID            string `json:"tenant_id,omitempty"`
	ControlPlaneBaseURL string `json:"control_plane_base_url,omitempty"`
}

type adminProfileView struct {
	ID                  string `json:"id"`
	KeyConfigured       bool   `json:"key_configured"`
	OrganizationID      string `json:"organization_id,omitempty"`
	WorkspaceID         string `json:"workspace_id,omitempty"`
	TenantID            string `json:"tenant_id,omitempty"`
	ReadAccess          bool   `json:"read_access"`
	ManageAccess        bool   `json:"manage_access"`
	ControlPlaneBaseURL string `json:"control_plane_base_url,omitempty"`
}

func profileView(admin tunnel.AdminConfig) adminProfileView {
	return adminProfileView{ID: admin.ID, KeyConfigured: admin.AdminKey != "", OrganizationID: admin.OrganizationID, WorkspaceID: admin.WorkspaceID, TenantID: admin.TenantID, ReadAccess: admin.ReadAccess, ManageAccess: admin.ManageAccess, ControlPlaneBaseURL: admin.ControlPlaneBaseURL}
}

func appProfileView(item application.TunnelAdminProfile) adminProfileView {
	return adminProfileView{ID: item.ID, KeyConfigured: item.KeyConfigured, OrganizationID: item.OrganizationID, WorkspaceID: item.WorkspaceID, TenantID: item.TenantID, ReadAccess: item.ReadAccess, ManageAccess: item.ManageAccess, ControlPlaneBaseURL: item.ControlPlaneBaseURL}
}

func profileFromRequest(request adminProfileRequest) tunnel.AdminConfig {
	return tunnel.AdminConfig{ID: strings.TrimSpace(request.ID), AdminKey: strings.TrimSpace(request.AdminKey), OrganizationID: strings.TrimSpace(request.OrganizationID), WorkspaceID: strings.TrimSpace(request.WorkspaceID), TenantID: strings.TrimSpace(request.TenantID), ControlPlaneBaseURL: strings.TrimSpace(request.ControlPlaneBaseURL)}
}

func (api API) handleTunnelAdmins(w http.ResponseWriter, r *http.Request) {
	_, collection, err := api.collectionState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		views := make([]adminProfileView, 0, len(collection.Admins))
		for _, admin := range collection.Admins {
			views = append(views, profileView(admin))
		}
		writeJSON(w, views)
	case http.MethodPost:
		var request adminProfileRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item, _, err := application.AddTunnelAdminProfile(r.Context(), profileFromRequest(request))
		if err != nil {
			writeTunnelCollectionError(w, err)
			return
		}
		if err := api.reloadCollectionFromDisk(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, appProfileView(item))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) handleTunnelAdmin(w http.ResponseWriter, r *http.Request) {
	_, collection, err := api.collectionState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tunnel-admins/"), "/")
	parts := strings.Split(path, "/")
	id := parts[0]
	index := -1
	for i, admin := range collection.Admins {
		if admin.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		http.Error(w, "admin profile not found", http.StatusNotFound)
		return
	}
	if len(parts) == 2 && parts[1] == "verify" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, _, err := application.VerifyTunnelAdminProfile(r.Context(), id)
		if err != nil {
			writeTunnelCollectionError(w, err)
			return
		}
		if err := api.reloadCollectionFromDisk(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, appProfileView(item))
		return
	}
	if len(parts) != 1 {
		http.Error(w, "unknown admin profile action", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, profileView(collection.Admins[index]))
	case http.MethodPut:
		var request adminProfileRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if request.ID != "" && request.ID != id {
			http.Error(w, "admin profile id cannot be changed", http.StatusBadRequest)
			return
		}
		admin := profileFromRequest(request)
		admin.ID = id
		item, _, err := application.UpdateTunnelAdminProfile(r.Context(), admin)
		if err != nil {
			writeTunnelCollectionError(w, err)
			return
		}
		if err := api.reloadCollectionFromDisk(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, appProfileView(item))
	case http.MethodDelete:
		if err := application.RemoveTunnelAdminProfile(r.Context(), id); err != nil {
			writeTunnelCollectionError(w, err)
			return
		}
		if err := api.reloadCollectionFromDisk(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type managedTunnelView struct {
	Metadata      tunnel.Metadata `json:"metadata"`
	AdminProfiles []string        `json:"admin_profiles"`
}

func managedView(item application.ManagedTunnelDiscovery) managedTunnelView {
	return managedTunnelView{Metadata: item.Metadata, AdminProfiles: item.AdminProfiles}
}

func writeManagedTunnelError(w http.ResponseWriter, err error) {
	msg := err.Error()
	code := http.StatusBadGateway
	switch {
	case strings.Contains(msg, "lacks"):
		code = http.StatusForbidden
	case strings.Contains(msg, "detach local"):
		code = http.StatusConflict
	case strings.Contains(msg, "not found"):
		code = http.StatusNotFound
	case strings.Contains(msg, "must be specified"):
		code = http.StatusBadRequest
	}
	http.Error(w, msg, code)
}

func (api API) handleManagedTunnelCollection(w http.ResponseWriter, r *http.Request) {
	profileID := strings.TrimSpace(r.URL.Query().Get("admin"))
	switch r.Method {
	case http.MethodPost:
		var request managedTunnelCreateRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item, err := application.CreateManagedTunnelByProfile(r.Context(), profileID, tunnel.CreateRequest{Name: request.Name, Description: request.Description, TenantIDs: request.TenantIDs, WorkspaceIDs: request.WorkspaceIDs, OrganizationIDs: request.OrganizationIDs})
		if err != nil {
			writeManagedTunnelError(w, err)
			return
		}
		writeJSON(w, managedView(item))
	case http.MethodGet:
		items, err := application.DiscoverManagedTunnels(r.Context(), profileID)
		if err != nil {
			writeManagedTunnelError(w, err)
			return
		}
		views := make([]managedTunnelView, 0, len(items))
		for _, item := range items {
			views = append(views, managedView(item))
		}
		writeJSON(w, views)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) handleManagedTunnelCollectionItem(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/managed-tunnels/"), "/")
	if path == "" || strings.Contains(path, "/") {
		http.Error(w, "managed tunnel id is required", http.StatusBadRequest)
		return
	}
	profileID := strings.TrimSpace(r.URL.Query().Get("admin"))
	switch r.Method {
	case http.MethodGet:
		item, err := application.GetManagedTunnelByProfile(r.Context(), path, profileID)
		if err != nil {
			writeManagedTunnelError(w, err)
			return
		}
		writeJSON(w, managedView(item))
	case http.MethodPut:
		var request managedTunnelUpdateRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item, err := application.UpdateManagedTunnelByProfile(r.Context(), path, profileID, tunnel.UpdateRequest{Name: request.Name, Description: request.Description, TenantIDs: request.TenantIDs, WorkspaceIDs: request.WorkspaceIDs, OrganizationIDs: request.OrganizationIDs})
		if err != nil {
			writeManagedTunnelError(w, err)
			return
		}
		writeJSON(w, managedView(item))
	case http.MethodDelete:
		metadata, err := application.DeleteManagedTunnelByProfile(r.Context(), path, profileID)
		if err != nil {
			writeManagedTunnelError(w, err)
			return
		}
		writeJSON(w, managedTunnelView{Metadata: metadata, AdminProfiles: []string{profileID}})
	case http.MethodPost:
		var request struct {
			RuntimeAPIKey          string `json:"runtime_api_key"`
			AutoGenerateRuntimeKey bool   `json:"auto_generate_runtime_key"`
			ProjectID              string `json:"project_id"`
			Enabled                bool   `json:"enabled"`
		}
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item, err := application.AttachManagedTunnelWithOptions(r.Context(), path, application.AttachManagedTunnelOptions{AdminProfileID: profileID, RuntimeAPIKey: request.RuntimeAPIKey, AutoGenerateRuntimeKey: request.AutoGenerateRuntimeKey, ProjectID: request.ProjectID, Enabled: request.Enabled})
		if err != nil {
			writeTunnelCollectionError(w, err)
			return
		}
		if err := api.reloadCollectionFromDisk(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		view, err := api.liveLocalView(item.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, view)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
