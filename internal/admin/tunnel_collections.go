package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
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

func localView(instance tunnel.InstanceConfig, status tunnel.Status) localTunnelView {
	status.AdminKeyConfigured = false
	status.AdminScope = nil
	return localTunnelView{ID: instance.ID, Enabled: instance.Enabled, RuntimeKeyConfigured: instance.APIKey != "", AdminProfileID: instance.AdminProfileID, ControlPlaneBaseURL: instance.ControlPlaneBaseURL, OrganizationID: instance.OrganizationID, Status: status}
}

func (api API) collectionState() (config.Config, tunnel.CollectionConfig, error) {
	if api.Config == nil || api.Tunnels == nil {
		return config.Config{}, tunnel.CollectionConfig{}, errors.New("tunnel runtime unavailable")
	}
	cfg := api.Config.Snapshot()
	return cfg, cfg.RuntimeTunnels(), nil
}

func setCollection(cfg *config.Config, collection tunnel.CollectionConfig) {
	instances := append([]tunnel.InstanceConfig{}, collection.Instances...)
	admins := append([]tunnel.AdminConfig{}, collection.Admins...)
	cfg.Tunnel = tunnel.Config{Instances: &instances, Admins: &admins}
}

func (api API) persistTunnelCollection(previous, next config.Config) error {
	if err := config.Validate(next); err != nil {
		return err
	}
	if err := api.persistConfig(next); err != nil {
		return err
	}
	if api.ReloadConfig != nil {
		if err := api.ReloadConfig(next); err != nil {
			return errors.Join(err, api.persistConfig(previous))
		}
		return nil
	}
	if err := api.Tunnels.Reconcile(context.Background(), next.RuntimeTunnels()); err != nil {
		return errors.Join(err, api.persistConfig(previous))
	}
	_, err := api.Config.Update(func(config.Config) (config.Config, error) { return next, nil })
	if err != nil {
		return errors.Join(err, api.persistConfig(previous), api.Tunnels.Reconcile(context.Background(), previous.RuntimeTunnels()))
	}
	return nil
}

func (api API) handleLocalTunnels(w http.ResponseWriter, r *http.Request) {
	cfg, collection, err := api.collectionState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		views := make([]localTunnelView, 0, len(collection.Instances))
		for _, instance := range collection.Instances {
			client, ok := api.Tunnels.Client(instance.ID)
			status := tunnel.Status{ID: instance.ID, Enabled: instance.Enabled}
			if ok {
				status = client.Status()
			}
			views = append(views, localView(instance, status))
		}
		writeJSON(w, views)
	case http.MethodPost:
		var request localTunnelRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		request.ID = strings.TrimSpace(request.ID)
		if request.ID == "" {
			http.Error(w, "tunnel id is required", http.StatusBadRequest)
			return
		}
		for _, instance := range collection.Instances {
			if instance.ID == request.ID {
				http.Error(w, "tunnel is already attached", http.StatusConflict)
				return
			}
		}
		instance := tunnel.InstanceConfig{ID: request.ID, Enabled: request.Enabled, APIKey: strings.TrimSpace(request.APIKey), AdminProfileID: strings.TrimSpace(request.AdminProfileID), ControlPlaneBaseURL: strings.TrimSpace(request.ControlPlaneBaseURL), OrganizationID: strings.TrimSpace(request.OrganizationID)}
		collection.Instances = append(collection.Instances, instance)
		next := cfg
		setCollection(&next, collection)
		if err := api.persistTunnelCollection(cfg, next); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		client, _ := api.Tunnels.Client(instance.ID)
		writeJSON(w, localView(instance, client.Status()))
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
	client, ok := api.Tunnels.Client(id)
	if !ok {
		http.Error(w, "tunnel client unavailable", http.StatusServiceUnavailable)
		return
	}
	if len(parts) == 2 {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch parts[1] {
		case "start":
			err = api.Tunnels.Start(r.Context(), id)
		case "stop":
			if !cfg.Server.Enabled && !anotherReadyTunnel(api.Tunnels.Statuses(), id) {
				http.Error(w, "cannot stop the last usable MCP transport", http.StatusConflict)
				return
			}
			err = api.Tunnels.Stop(r.Context(), id)
		case "enable", "disable":
			collection.Instances[index].Enabled = parts[1] == "enable"
			next := cfg
			setCollection(&next, collection)
			err = api.persistTunnelCollection(cfg, next)
		default:
			http.Error(w, "unknown tunnel action", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		client, _ = api.Tunnels.Client(id)
		writeJSON(w, localView(collection.Instances[index], client.Status()))
		return
	}
	if len(parts) != 1 {
		http.Error(w, "unknown tunnel path", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, localView(collection.Instances[index], client.Status()))
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
		if instance.APIKey == "" {
			instance.APIKey = collection.Instances[index].APIKey
		}
		collection.Instances[index] = instance
		next := cfg
		setCollection(&next, collection)
		if err := api.persistTunnelCollection(cfg, next); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		client, _ = api.Tunnels.Client(id)
		writeJSON(w, localView(instance, client.Status()))
	case http.MethodDelete:
		collection.Instances = append(collection.Instances[:index], collection.Instances[index+1:]...)
		next := cfg
		setCollection(&next, collection)
		if err := api.persistTunnelCollection(cfg, next); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func anotherReadyTunnel(statuses []tunnel.Status, except string) bool {
	for _, status := range statuses {
		if status.ID != except && status.Enabled && status.Ready {
			return true
		}
	}
	return false
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

func profileFromRequest(request adminProfileRequest) tunnel.AdminConfig {
	return tunnel.AdminConfig{ID: strings.TrimSpace(request.ID), AdminKey: strings.TrimSpace(request.AdminKey), OrganizationID: strings.TrimSpace(request.OrganizationID), WorkspaceID: strings.TrimSpace(request.WorkspaceID), TenantID: strings.TrimSpace(request.TenantID), ControlPlaneBaseURL: strings.TrimSpace(request.ControlPlaneBaseURL)}
}

func (api API) handleTunnelAdmins(w http.ResponseWriter, r *http.Request) {
	cfg, collection, err := api.collectionState()
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
		admin := profileFromRequest(request)
		if admin.ID == "" {
			http.Error(w, "admin profile id is required", http.StatusBadRequest)
			return
		}
		for _, existing := range collection.Admins {
			if existing.ID == admin.ID {
				http.Error(w, "admin profile already exists", http.StatusConflict)
				return
			}
		}
		collection.Admins = append(collection.Admins, admin)
		next := cfg
		setCollection(&next, collection)
		if err := api.persistTunnelCollection(cfg, next); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, profileView(admin))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) handleTunnelAdmin(w http.ResponseWriter, r *http.Request) {
	cfg, collection, err := api.collectionState()
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
		access, _, err := tunnel.VerifyAdminProfile(r.Context(), collection.Admins[index])
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		collection.Admins[index].ReadAccess, collection.Admins[index].ManageAccess = access.Read, access.Manage
		next := cfg
		setCollection(&next, collection)
		if err := api.persistTunnelCollection(cfg, next); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, profileView(collection.Admins[index]))
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
		if admin.AdminKey == "" {
			admin.AdminKey = collection.Admins[index].AdminKey
			admin.ReadAccess = collection.Admins[index].ReadAccess
			admin.ManageAccess = collection.Admins[index].ManageAccess
		}
		collection.Admins[index] = admin
		next := cfg
		setCollection(&next, collection)
		if err := api.persistTunnelCollection(cfg, next); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, profileView(admin))
	case http.MethodDelete:
		for _, instance := range collection.Instances {
			if instance.AdminProfileID == id {
				http.Error(w, "admin profile is used by a local tunnel", http.StatusConflict)
				return
			}
		}
		collection.Admins = append(collection.Admins[:index], collection.Admins[index+1:]...)
		next := cfg
		setCollection(&next, collection)
		if err := api.persistTunnelCollection(cfg, next); err != nil {
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

func (api API) handleManagedTunnelCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		_, collection, err := api.collectionState()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		admin, err := resolveAdminProfile(collection, r.URL.Query().Get("admin"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !admin.ManageAccess {
			http.Error(w, "admin profile lacks Manage access", http.StatusForbidden)
			return
		}
		var request managedTunnelCreateRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		metadata, err := tunnel.CreateManagedForAdmin(r.Context(), admin, tunnel.CreateRequest{Name: request.Name, Description: request.Description, TenantIDs: request.TenantIDs, WorkspaceIDs: request.WorkspaceIDs, OrganizationIDs: request.OrganizationIDs})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		_, _ = config.SaveTunnelMetadata(metadata)
		writeJSON(w, managedTunnelView{Metadata: metadata, AdminProfiles: []string{admin.ID}})
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_, collection, err := api.collectionState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	profileID := strings.TrimSpace(r.URL.Query().Get("admin"))
	items := make(map[string]managedTunnelView)
	order := []string{}
	for _, admin := range collection.Admins {
		if profileID != "" && admin.ID != profileID {
			continue
		}
		if !admin.ReadAccess && !admin.ManageAccess {
			continue
		}
		found, err := tunnel.ListManagedForAdmin(r.Context(), admin)
		if err != nil {
			http.Error(w, fmt.Sprintf("admin profile %s: %v", admin.ID, err), http.StatusBadGateway)
			return
		}
		for _, metadata := range found {
			item, ok := items[metadata.ID]
			if !ok {
				item = managedTunnelView{Metadata: metadata}
				order = append(order, metadata.ID)
			}
			item.AdminProfiles = append(item.AdminProfiles, admin.ID)
			items[metadata.ID] = item
		}
	}
	if profileID != "" {
		found := false
		for _, admin := range collection.Admins {
			if admin.ID == profileID {
				found = true
			}
		}
		if !found {
			http.Error(w, "admin profile not found", http.StatusNotFound)
			return
		}
	}
	views := make([]managedTunnelView, 0, len(order))
	for _, id := range order {
		views = append(views, items[id])
	}
	writeJSON(w, views)
}

func resolveAdminProfile(collection tunnel.CollectionConfig, id string) (tunnel.AdminConfig, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		if len(collection.Admins) == 1 {
			return collection.Admins[0], nil
		}
		return tunnel.AdminConfig{}, errors.New("select an admin profile")
	}
	for _, admin := range collection.Admins {
		if admin.ID == id {
			return admin, nil
		}
	}
	return tunnel.AdminConfig{}, fmt.Errorf("admin profile %q not found", id)
}

func (api API) handleManagedTunnelCollectionItem(w http.ResponseWriter, r *http.Request) {
	cfg, collection, err := api.collectionState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/managed-tunnels/"), "/")
	if path == "" || strings.Contains(path, "/") {
		http.Error(w, "managed tunnel id is required", http.StatusBadRequest)
		return
	}
	admin, err := resolveAdminProfile(collection, r.URL.Query().Get("admin"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !admin.ReadAccess && !admin.ManageAccess {
			http.Error(w, "admin profile lacks Read access", http.StatusForbidden)
			return
		}
		metadata, err := tunnel.GetManagedForAdmin(r.Context(), admin, path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, managedTunnelView{Metadata: metadata, AdminProfiles: []string{admin.ID}})
	case http.MethodPut:
		if !admin.ManageAccess {
			http.Error(w, "admin profile lacks Manage access", http.StatusForbidden)
			return
		}
		var request managedTunnelUpdateRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		metadata, err := tunnel.UpdateManagedForAdmin(r.Context(), admin, path, tunnel.UpdateRequest{Name: request.Name, Description: request.Description, TenantIDs: request.TenantIDs, WorkspaceIDs: request.WorkspaceIDs, OrganizationIDs: request.OrganizationIDs})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		_, _ = config.SaveTunnelMetadata(metadata)
		writeJSON(w, managedTunnelView{Metadata: metadata, AdminProfiles: []string{admin.ID}})
	case http.MethodDelete:
		if !admin.ManageAccess {
			http.Error(w, "admin profile lacks Manage access", http.StatusForbidden)
			return
		}
		for _, instance := range collection.Instances {
			if instance.ID == path {
				http.Error(w, "detach local tunnel before remote deletion", http.StatusConflict)
				return
			}
		}
		metadata, err := tunnel.DeleteManagedForAdmin(r.Context(), admin, path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		_ = config.RemoveTunnelMetadata(metadata.ID)
		writeJSON(w, managedTunnelView{Metadata: metadata, AdminProfiles: []string{admin.ID}})
	case http.MethodPost:
		if !admin.ReadAccess && !admin.ManageAccess {
			http.Error(w, "admin profile lacks Read access", http.StatusForbidden)
			return
		}
		for _, instance := range collection.Instances {
			if instance.ID == path {
				http.Error(w, "tunnel is already attached", http.StatusConflict)
				return
			}
		}
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
		metadata, err := tunnel.GetManagedForAdmin(r.Context(), admin, path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		key := strings.TrimSpace(request.RuntimeAPIKey)
		if key == "" && request.AutoGenerateRuntimeKey {
			if !admin.ManageAccess {
				http.Error(w, "admin profile lacks Manage access", http.StatusForbidden)
				return
			}
			generated, err := tunnel.GenerateRuntimeKeyForAdmin(r.Context(), admin, request.ProjectID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			key = generated.Value
		}
		if key == "" {
			http.Error(w, "runtime API key is required", http.StatusBadRequest)
			return
		}
		instance := tunnel.InstanceConfig{ID: path, Enabled: request.Enabled, APIKey: key, AdminProfileID: admin.ID, ControlPlaneBaseURL: admin.ControlPlaneBaseURL}
		if len(metadata.OrganizationIDs) > 0 {
			instance.OrganizationID = metadata.OrganizationIDs[0]
		}
		collection.Instances = append(collection.Instances, instance)
		next := cfg
		setCollection(&next, collection)
		if err := api.persistTunnelCollection(cfg, next); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = config.SaveTunnelMetadata(metadata)
		client, _ := api.Tunnels.Client(path)
		writeJSON(w, localView(instance, client.Status()))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
