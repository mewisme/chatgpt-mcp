package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/secretstore"
	"go.mewis.me/chatgpt-mcp/internal/state"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type tunnelSecret struct {
	RuntimeKeyConfigured bool   `json:"runtime_key_configured,omitempty"`
	AdminKeyConfigured   bool   `json:"admin_key_configured,omitempty"`
	APIKey               string `json:"api_key,omitempty"`
	AdminKey             string `json:"admin_key,omitempty"`
	AdminOrganizationID  string `json:"admin_organization_id,omitempty"`
	AdminWorkspaceID     string `json:"admin_workspace_id,omitempty"`
	AdminTenantID        string `json:"admin_tenant_id,omitempty"`
}

var (
	tunnelRuntimeSecretName = secretstore.Name("tunnel", "runtime-key")
	tunnelAdminSecretName   = secretstore.Name("tunnel", "admin-key")
)

func TunnelSecretPath() string { return configformat.StructuredPath(RootPath(), "tunnel") }

func TunnelKeyringEntries(root string) ([]string, error) {
	stored, err := loadTunnelSecretAt(configformat.StructuredPath(root, "tunnel"))
	if err != nil {
		return nil, err
	}
	entries := []string{}
	if stored.RuntimeKeyConfigured {
		entries = append(entries, tunnelRuntimeSecretName)
	}
	if stored.AdminKeyConfigured {
		entries = append(entries, tunnelAdminSecretName)
	}
	return entries, nil
}

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

func loadTunnelSecrets(path string, cfg *tunnel.Config, legacyRuntime, legacyAdmin string) (bool, error) {
	stored, err := loadTunnelSecretAt(path)
	if err != nil {
		return false, err
	}
	if stored.AdminOrganizationID != "" || stored.AdminWorkspaceID != "" || stored.AdminTenantID != "" {
		cfg.AdminOrganizationID = stored.AdminOrganizationID
		cfg.AdminWorkspaceID = stored.AdminWorkspaceID
		cfg.AdminTenantID = stored.AdminTenantID
	}
	if stored.APIKey != "" {
		legacyRuntime = stored.APIKey
	}
	if stored.AdminKey != "" {
		legacyAdmin = stored.AdminKey
	}
	store := secretstore.New(filepath.Dir(path))
	runtimeKey, runtimeMigration, err := resolveKeyringSecret(store, tunnelRuntimeSecretName, stored.RuntimeKeyConfigured, legacyRuntime, "tunnel runtime key")
	if err != nil {
		return false, err
	}
	adminKey, adminMigration, err := resolveKeyringSecret(store, tunnelAdminSecretName, stored.AdminKeyConfigured, legacyAdmin, "tunnel admin key")
	if err != nil {
		return false, err
	}
	cfg.APIKey = runtimeKey
	cfg.AdminKey = adminKey
	return runtimeMigration || adminMigration || stored.APIKey != "" || stored.AdminKey != "", nil
}

func resolveKeyringSecret(store *secretstore.Store, name string, configured bool, legacy, label string) (string, bool, error) {
	if configured {
		value, err := store.Get(name)
		if err == nil {
			return value, legacy != "", nil
		}
		if !errors.Is(err, secretstore.ErrNotFound) {
			return "", false, fmt.Errorf("load %s from OS keyring: %w", label, err)
		}
		if legacy == "" {
			return "", false, fmt.Errorf("%s is configured but missing from OS keyring", label)
		}
	}
	if legacy != "" {
		return legacy, true, nil
	}
	return "", false, nil
}

func saveTunnelSecretAt(path string, cfg tunnel.Config) error {
	previous, err := loadTunnelSecretAt(path)
	if err != nil {
		return err
	}
	stored := tunnelSecret{
		RuntimeKeyConfigured: cfg.APIKey != "", AdminKeyConfigured: cfg.AdminKey != "",
		AdminOrganizationID: cfg.AdminOrganizationID, AdminWorkspaceID: cfg.AdminWorkspaceID, AdminTenantID: cfg.AdminTenantID,
	}
	if stored.RuntimeKeyConfigured || stored.AdminKeyConfigured || stored.AdminOrganizationID != "" || stored.AdminWorkspaceID != "" || stored.AdminTenantID != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		data, err := configformat.MarshalPath(path, stored)
		if err != nil {
			return err
		}
		if err := state.WriteFileAtomic(path, data, 0600); err != nil {
			return err
		}
	} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	changes := make([]secretstore.Change, 0, 2)
	if stored.RuntimeKeyConfigured || previous.RuntimeKeyConfigured || previous.APIKey != "" {
		changes = append(changes, secretstore.Change{Name: tunnelRuntimeSecretName, Value: cfg.APIKey})
	}
	if stored.AdminKeyConfigured || previous.AdminKeyConfigured || previous.AdminKey != "" {
		changes = append(changes, secretstore.Change{Name: tunnelAdminSecretName, Value: cfg.AdminKey})
	}
	return secretstore.New(filepath.Dir(path)).Apply(changes)
}
