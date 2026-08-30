package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/logger"
)

func configCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Read and update validated runtime configuration"}
	cmd.AddCommand(
		&cobra.Command{Use: "path", RunE: func(cmd *cobra.Command, args []string) error {
			source, err := config.Source()
			if err != nil {
				return err
			}
			log := logger.NewCLIWithWriter(cmd.OutOrStdout())
			log.Detail("config", source.Path)
			log.Detail("format", source.Format)
			log.Detail("root", config.RootPath())
			return nil
		}},
		configGetCommand(),
		configListCommand(),
		configSetCommand(),
		configConvertCommand(),
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
	options := configOutputOptions{}
	cmd := &cobra.Command{
		Use:   "get [key]",
		Short: "Get a redacted config value or subtree",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			key := ""
			if len(args) > 0 {
				key = args[0]
			}
			return printConfigSelection(cmd, cfg, key, false, options)
		},
	}
	addConfigOutputFlags(cmd, &options)
	return cmd
}

func configListCommand() *cobra.Command {
	options := configOutputOptions{}
	cmd := &cobra.Command{
		Use:   "list [key]",
		Short: "List redacted configuration with optional subtree and output format",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			key := ""
			if len(args) > 0 {
				key = args[0]
			}
			return printConfigSelection(cmd, cfg, key, true, options)
		},
	}
	addConfigOutputFlags(cmd, &options)
	return cmd
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
	tree, err := redactedConfigTree(cfg)
	if err != nil {
		return nil, err
	}
	return getConfigTreeValue(tree, key)
}

func configConvertCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "convert <json|yaml|toml>",
		Short: "Convert main config and all structured chatgpt-mcp state files to one format",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := configformat.Parse(args[0])
			if err != nil {
				return err
			}
			converted, err := config.ConvertFormat(format)
			if err != nil {
				return err
			}
			log := logger.NewCLIWithWriter(cmd.OutOrStdout())
			log.Success("CONFIG", "configuration format converted", "format", format, "files", converted)
			log.Detail("config", config.PathForFormat(format))
			return nil
		},
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
