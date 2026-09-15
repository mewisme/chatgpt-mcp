package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestTunnelCollectionSecretsAcrossFormats(t *testing.T) {
	for _, format := range []configformat.Format{configformat.JSON, configformat.YAML, configformat.TOML} {
		t.Run(string(format), func(t *testing.T) {
			root := t.TempDir()
			configPath := filepath.Join(root, "config"+configformat.Extension(format))
			secretPath := filepath.Join(root, "tunnel"+configformat.Extension(format))
			cfg := Default()
			instances := []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_a", APIKey: "runtime-a"}, {ID: "tunnel_b", APIKey: "runtime-b", AdminProfileID: "default"}}
			admins := []tunnel.AdminConfig{{ID: "default", AdminKey: "admin-secret"}}
			cfg.Tunnel.Instances, cfg.Tunnel.Admins = &instances, &admins
			if err := saveAt(configPath, secretPath, cfg); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "runtime-a") || strings.Contains(string(data), "runtime-b") || strings.Contains(string(data), "admin-secret") {
				t.Fatalf("config exposed keys: %s", data)
			}
			loaded, err := loadAt(configPath, secretPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := loaded.Tunnel.Collection().Instances[0].APIKey; got != "runtime-a" {
				t.Fatalf("runtime key = %q", got)
			}
			if got := loaded.Tunnel.Collection().Instances[1].APIKey; got != "runtime-b" {
				t.Fatalf("second runtime key = %q", got)
			}
			if got := loaded.Tunnel.Collection().Admins[0].AdminKey; got != "admin-secret" {
				t.Fatalf("admin key = %q", got)
			}
		})
	}
}
