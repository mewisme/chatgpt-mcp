package config

import (
	"os"
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/state"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type tunnelSecret struct {
	APIKey              string `json:"api_key,omitempty"`
	AdminKey            string `json:"admin_key,omitempty"`
	AdminOrganizationID string `json:"admin_organization_id,omitempty"`
	AdminWorkspaceID    string `json:"admin_workspace_id,omitempty"`
	AdminTenantID       string `json:"admin_tenant_id,omitempty"`
}

func TunnelSecretPath() string { return configformat.StructuredPath(RootPath(), "tunnel") }

func loadTunnelSecretAt(path string) (tunnelSecret, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tunnelSecret{}, nil
		}
		return tunnelSecret{}, err
	}
	var secret tunnelSecret
	if err := configformat.UnmarshalPath(path, data, &secret); err != nil {
		return tunnelSecret{}, err
	}
	return secret, nil
}

func saveTunnelSecretAt(path string, cfg tunnel.Config) error {
	secret := tunnelSecret{
		APIKey: cfg.APIKey, AdminKey: cfg.AdminKey,
		AdminOrganizationID: cfg.AdminOrganizationID, AdminWorkspaceID: cfg.AdminWorkspaceID, AdminTenantID: cfg.AdminTenantID,
	}
	if secret.APIKey == "" && secret.AdminKey == "" && secret.AdminOrganizationID == "" && secret.AdminWorkspaceID == "" && secret.AdminTenantID == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := configformat.MarshalPath(path, secret)
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(path, data, 0600)
}
