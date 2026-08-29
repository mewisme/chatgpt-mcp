package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/state"
)

type tunnelSecret struct {
	APIKey string `json:"api_key,omitempty"`
}

func TunnelSecretPath() string { return filepath.Join(RootPath(), "tunnel.json") }

func loadTunnelSecretAt(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var secret tunnelSecret
	if err := json.Unmarshal(data, &secret); err != nil {
		return "", err
	}
	return secret.APIKey, nil
}

func saveTunnelSecretAt(path, apiKey string) error {
	if apiKey == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tunnelSecret{APIKey: apiKey}, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(path, append(data, '\n'), 0600)
}
