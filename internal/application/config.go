package application

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	mcpoauth "go.mewis.me/chatgpt-mcp/internal/oauth"
	"go.mewis.me/chatgpt-mcp/internal/secretstore"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

type InitOptions struct {
	Force          bool
	Format         configformat.Format
	FormatSelected bool
}

type InitResult struct {
	Config     config.Config
	ConfigPath string
	Format     configformat.Format
	MCPToken   string
	AdminToken string
}

func Initialize(options InitOptions) (InitResult, error) {
	source, err := config.Source()
	if err != nil {
		return InitResult{}, err
	}
	format := options.Format
	if !options.FormatSelected {
		format = configformat.JSON
		if source.Exists {
			format = source.Format
		}
	}
	if source.Exists && !options.Force {
		return InitResult{}, errors.New("configuration already exists; use --force to rotate tokens and rewrite config")
	}
	if source.Exists && options.FormatSelected && format != source.Format {
		return InitResult{}, fmt.Errorf("cannot change storage format with init --force; convert configuration to %s first", format)
	}
	cfg := config.Default()
	mcpToken := auth.GenerateToken("mcp")
	adminToken := auth.GenerateToken("admin")
	cfg.Auth.MCPTokenHash = auth.HashToken(mcpToken)
	cfg.Auth.AdminTokenHash = auth.HashToken(adminToken)
	if err := config.Validate(cfg); err != nil {
		return InitResult{}, err
	}
	path := source.Path
	if source.Exists {
		if err := config.Save(cfg); err != nil {
			return InitResult{}, err
		}
	} else {
		if err := config.SaveAs(cfg, format); err != nil {
			return InitResult{}, err
		}
		path = config.PathForFormat(format)
	}
	return InitResult{Config: cfg, ConfigPath: path, Format: format, MCPToken: mcpToken, AdminToken: adminToken}, nil
}

func Uninitialize(root string) error {
	if err := PurgeStoredSecrets(root); err != nil {
		return err
	}
	return RemoveConfigRoot(root)
}

func PurgeStoredSecrets(root string) error {
	entries, err := config.TunnelSecretEntries(root)
	if err != nil {
		return err
	}
	oauthEntries, err := mcpoauth.NewStore(configformat.StructuredPath(root, "oauth")).SecretEntries()
	if err != nil {
		return err
	}
	upstreamEntries, err := upstream.NewStore(configformat.StructuredPath(root, "upstream")).SecretEntries()
	if err != nil {
		return err
	}
	entries = append(entries, secretstore.Name("cluster", "relay-token"))
	entries = append(entries, oauthEntries...)
	entries = append(entries, upstreamEntries...)
	changes := make([]secretstore.Change, 0, len(entries))
	for _, entry := range entries {
		changes = append(changes, secretstore.Change{Name: entry})
	}
	return secretstore.New(root).Apply(changes)
}

func RemoveConfigRoot(root string) error {
	clean := filepath.Clean(root)
	if clean == "." || clean == string(filepath.Separator) {
		return fmt.Errorf("refusing to remove unsafe config root: %s", clean)
	}
	volume := filepath.VolumeName(clean)
	if clean == volume+string(filepath.Separator) {
		return fmt.Errorf("refusing to remove volume root: %s", clean)
	}
	if clean != filepath.Clean(configformat.DefaultRootPath()) && !configformat.IsManagedRoot(clean) {
		return fmt.Errorf("refusing to remove unmanaged config root: %s", clean)
	}
	return os.RemoveAll(clean)
}

func MigrateLegacySecrets() error {
	if _, err := config.Load(); err != nil {
		return err
	}
	if _, err := upstream.NewStore(upstream.Path()).Load(); err != nil {
		return err
	}
	return mcpoauth.NewStore(mcpoauth.Path()).Migrate()
}
