package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/secretstore"
)

var clusterRelayTokenSecretName = secretstore.Name("cluster", "relay-token")

func ClusterKeyringEntries(root string) ([]string, error) {
	source, err := configformat.Discover(root)
	if err != nil {
		return nil, err
	}
	if !source.Exists {
		return nil, nil
	}
	data, err := os.ReadFile(source.Path)
	if err != nil {
		return nil, err
	}
	var stored struct {
		Cluster ClusterConfig `json:"cluster"`
	}
	if err := configformat.UnmarshalPath(source.Path, data, &stored); err != nil {
		return nil, err
	}
	if stored.Cluster.RelayTokenConfigured || stored.Cluster.RelayToken != "" {
		return []string{clusterRelayTokenSecretName}, nil
	}
	return nil, nil
}

func loadClusterSecret(root string, cfg *ClusterConfig, legacy string) (bool, error) {
	if cfg == nil {
		return false, errors.New("cluster config is required")
	}
	value, migrate, err := resolveKeyringSecret(secretstore.New(root), clusterRelayTokenSecretName, cfg.RelayTokenConfigured, legacy, "cluster relay token")
	if err != nil {
		return false, err
	}
	cfg.RelayToken = value
	cfg.RelayTokenConfigured = value != ""
	return migrate, nil
}

func saveClusterSecret(root string, cfg ClusterConfig) error {
	store := secretstore.New(filepath.Clean(root))
	if err := store.Apply([]secretstore.Change{{Name: clusterRelayTokenSecretName, Value: cfg.RelayToken}}); err != nil {
		return fmt.Errorf("save cluster relay token to OS keyring: %w", err)
	}
	return nil
}
