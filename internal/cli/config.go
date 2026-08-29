package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
)

type configAuthView struct {
	MCPEnabled           bool `json:"mcp_enabled"`
	AdminEnabled         bool `json:"admin_enabled"`
	MCPTokenConfigured   bool `json:"mcp_token_configured"`
	AdminTokenConfigured bool `json:"admin_token_configured"`
}

type tunnelView struct {
	Enabled             bool   `json:"enabled"`
	ID                  string `json:"id,omitempty"`
	APIKeyConfigured    bool   `json:"api_key_configured"`
	ControlPlaneBaseURL string `json:"control_plane_base_url,omitempty"`
	OrganizationID      string `json:"organization_id,omitempty"`
}

type configView struct {
	Server config.ServerConfig `json:"server"`
	Admin  config.AdminConfig  `json:"admin"`
	Auth   configAuthView      `json:"auth"`
	Tunnel tunnelView          `json:"tunnel"`
}

func configCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Read and update validated runtime configuration"}
	cmd.AddCommand(
		&cobra.Command{Use: "path", Run: func(cmd *cobra.Command, args []string) {
			log := logger.NewCLIWithWriter(cmd.OutOrStdout())
			log.Detail("config", config.Path())
			log.Detail("root", config.RootPath())
		}},
		configGetCommand(),
		configSetCommand(),
		configPresetCommand(),
		&cobra.Command{Use: "validate", RunE: func(cmd *cobra.Command, args []string) error {
			value, err := config.Load()
			if err != nil {
				return err
			}
			if err := config.Validate(value); err != nil {
				return err
			}
			logger.NewCLIWithWriter(cmd.OutOrStdout()).Success("CONFIG", "configuration is valid")
			return nil
		}},
	)
	return cmd
}

func configGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get [key]",
		Short: "Show redacted configuration or one supported key",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				data, err := json.MarshalIndent(configPublicView(cfg), "", "  ")
				if err != nil {
					return err
				}
				cmd.Println(string(data))
				return nil
			}
			value, err := getConfigValue(cfg, args[0])
			if err != nil {
				return err
			}
			if text, ok := value.(string); ok {
				cmd.Println(text)
				return nil
			}
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		},
	}
}

func configSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set one typed configuration value; key=value is also accepted",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, raw, err := parseConfigSetArgs(args)
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := setConfigValue(&cfg, key, raw); err != nil {
				return err
			}
			if err := config.Validate(cfg); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			logger.NewCLIWithWriter(cmd.OutOrStdout()).Success("CONFIG", "value saved", "key", key)
			return nil
		},
	}
}

func configPresetCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "preset", Short: "List, inspect, and apply built-in configuration presets"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List built-in configuration presets",
			RunE: func(cmd *cobra.Command, args []string) error {
				log := logger.NewCLIWithWriter(cmd.OutOrStdout())
				presets := config.Presets()
				log.Success("PRESET", "configuration presets loaded", "count", len(presets))
				for _, preset := range presets {
					log.Detail(preset.Name, preset.Description)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "show <name>",
			Short: "Show one built-in configuration preset",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				preset, err := config.PresetByName(args[0])
				if err != nil {
					return err
				}
				return printJSON(cmd, preset)
			},
		},
		&cobra.Command{
			Use:     "apply <name>",
			Aliases: []string{"use"},
			Short:   "Apply a preset while preserving configured secrets and tunnel details",
			Args:    cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				if err := config.ApplyPreset(&cfg, args[0]); err != nil {
					return err
				}
				if err := config.Save(cfg); err != nil {
					return err
				}
				log := logger.NewCLIWithWriter(cmd.OutOrStdout())
				log.Success("PRESET", "configuration preset applied", "name", config.MatchPreset(cfg))
				logEndpointDetails(log, cfg)
				log.Detail("secrets", "preserved")
				return nil
			},
		},
		&cobra.Command{
			Use:   "current",
			Short: "Print the matching preset name or custom",
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				cmd.Println(config.MatchPreset(cfg))
				return nil
			},
		},
	)
	return cmd
}

func configPublicView(cfg config.Config) configView {
	return configView{
		Server: cfg.Server,
		Admin:  cfg.Admin,
		Auth: configAuthView{
			MCPEnabled: cfg.Auth.MCPEnabled, AdminEnabled: cfg.Auth.AdminEnabled,
			MCPTokenConfigured: cfg.Auth.MCPTokenHash != "", AdminTokenConfigured: cfg.Auth.AdminTokenHash != "",
		},
		Tunnel: tunnelView{
			Enabled: cfg.Tunnel.Enabled, ID: cfg.Tunnel.ID, APIKeyConfigured: cfg.Tunnel.APIKey != "",
			ControlPlaneBaseURL: cfg.Tunnel.ControlPlaneBaseURL, OrganizationID: cfg.Tunnel.OrganizationID,
		},
	}
}

func parseConfigSetArgs(args []string) (string, string, error) {
	if len(args) == 2 {
		key := strings.TrimSpace(args[0])
		if key == "" {
			return "", "", errors.New("config key is required")
		}
		return key, args[1], nil
	}
	key, value, ok := strings.Cut(args[0], "=")
	if !ok || strings.TrimSpace(key) == "" {
		return "", "", errors.New("use config set <key> <value> or config set key=value")
	}
	return strings.TrimSpace(key), value, nil
}

func setConfigValue(cfg *config.Config, key, raw string) error {
	switch key {
	case "server.expose":
		value, err := parseBool(raw, key)
		if err != nil {
			return err
		}
		cfg.Server.Expose = value
	case "server.port":
		value, err := parseInt(raw, key)
		if err != nil {
			return err
		}
		cfg.Server.Port = value
	case "admin.enabled":
		value, err := parseBool(raw, key)
		if err != nil {
			return err
		}
		cfg.Admin.Enabled = value
	case "admin.port":
		value, err := parseInt(raw, key)
		if err != nil {
			return err
		}
		cfg.Admin.Port = value
	case "auth.mcp_enabled":
		value, err := parseBool(raw, key)
		if err != nil {
			return err
		}
		cfg.Auth.MCPEnabled = value
	case "auth.admin_enabled":
		value, err := parseBool(raw, key)
		if err != nil {
			return err
		}
		cfg.Auth.AdminEnabled = value
	case "tunnel.enabled":
		value, err := parseBool(raw, key)
		if err != nil {
			return err
		}
		cfg.Tunnel.Enabled = value
	case "tunnel.id":
		cfg.Tunnel.ID = raw
	case "tunnel.api_key":
		cfg.Tunnel.APIKey = raw
	case "tunnel.control_plane_base_url":
		cfg.Tunnel.ControlPlaneBaseURL = raw
	case "tunnel.organization_id":
		cfg.Tunnel.OrganizationID = raw
	case "auth.mcp_token_hash", "auth.admin_token_hash":
		return errors.New("token hashes cannot be set through config; use chatgpt-mcp auth *-create")
	default:
		return fmt.Errorf("unsupported config key: %s", key)
	}
	return nil
}

func getConfigValue(cfg config.Config, key string) (any, error) {
	switch key {
	case "server.expose":
		return cfg.Server.Expose, nil
	case "server.port":
		return cfg.Server.Port, nil
	case "admin.enabled":
		return cfg.Admin.Enabled, nil
	case "admin.port":
		return cfg.Admin.Port, nil
	case "auth.mcp_enabled":
		return cfg.Auth.MCPEnabled, nil
	case "auth.admin_enabled":
		return cfg.Auth.AdminEnabled, nil
	case "auth.mcp_token_configured":
		return cfg.Auth.MCPTokenHash != "", nil
	case "auth.admin_token_configured":
		return cfg.Auth.AdminTokenHash != "", nil
	case "tunnel.enabled":
		return cfg.Tunnel.Enabled, nil
	case "tunnel.id":
		return cfg.Tunnel.ID, nil
	case "tunnel.api_key_configured":
		return cfg.Tunnel.APIKey != "", nil
	case "tunnel.control_plane_base_url":
		return cfg.Tunnel.ControlPlaneBaseURL, nil
	case "tunnel.organization_id":
		return cfg.Tunnel.OrganizationID, nil
	case "auth.mcp_token_hash", "auth.admin_token_hash", "tunnel.api_key":
		return nil, errors.New("sensitive config values cannot be read through the CLI")
	default:
		return nil, fmt.Errorf("unsupported config key: %s", key)
	}
}

func parseBool(raw, key string) (bool, error) {
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func parseInt(raw, key string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return value, nil
}
