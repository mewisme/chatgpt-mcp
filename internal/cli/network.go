package cli

import (
	"fmt"
	"net"
	"strconv"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	mcpnetwork "go.mewis.me/chatgpt-mcp/internal/network"
)

type listenerPlan struct {
	Hosts     []string
	Addresses []mcpnetwork.Address
}

func addExposeFlag(cmd *cobra.Command) {
	cmd.Flags().String("expose", "", "network exposure for this run: none, all, 0.0.0.0, or comma-separated interface names")
	if flag := cmd.Flags().Lookup("expose"); flag != nil {
		flag.NoOptDefVal = "all"
	}
}

func applyExposeOverride(cmd *cobra.Command, cfg *config.Config) error {
	flag := cmd.Flags().Lookup("expose")
	if flag == nil || !flag.Changed {
		return nil
	}
	raw, err := cmd.Flags().GetString("expose")
	if err != nil {
		return err
	}
	exposure, err := config.ParseExposure(raw)
	if err != nil {
		return err
	}
	cfg.Server.Expose = exposure
	return nil
}

func resolveListenerPlan(exposure config.ExposureConfig) (listenerPlan, error) {
	hosts, addresses, err := mcpnetwork.ResolveCurrent(exposure)
	if err != nil {
		return listenerPlan{}, err
	}
	return listenerPlan{Hosts: hosts, Addresses: addresses}, nil
}

func listenOnHosts(hosts []string, port int) ([]net.Listener, error) {
	listeners := make([]net.Listener, 0, len(hosts))
	for _, host := range hosts {
		listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			for _, opened := range listeners {
				_ = opened.Close()
			}
			return nil, fmt.Errorf("listen on %s:%d: %w", host, port, err)
		}
		listeners = append(listeners, listener)
	}
	return listeners, nil
}

func endpointURL(host string, port int, endpointPath string) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + endpointPath
}

func logReadyEndpoints(log *logger.Logger, cfg config.Config, plan listenerPlan) {
	mcpEndpoints := make([]string, 0, len(plan.Addresses))
	adminEndpoints := make([]string, 0, len(plan.Addresses))
	for _, address := range plan.Addresses {
		mcpEndpoints = append(mcpEndpoints, endpointURL(address.Host, cfg.Server.Port, "/mcp"))
		if cfg.Admin.Enabled {
			adminEndpoints = append(adminEndpoints, endpointURL(address.Host, cfg.Admin.Port, "/"))
		}
	}
	fields := []logger.Field{logger.With("mcp", mcpEndpoints)}
	if cfg.Admin.Enabled {
		fields = append(fields, logger.With("admin", adminEndpoints))
	}
	fields = append(fields, logger.WithVerbose("expose", cfg.Server.Expose.Mode))
	switch cfg.Server.Expose.Mode {
	case config.ExposureAll:
		fields = append(fields, logger.WithVerbose("network_addresses", max(0, len(plan.Addresses)-1)))
	case config.ExposureWildcard:
		fields = append(fields, logger.WithVerbose("bind", mcpnetwork.WildcardHost), logger.WithVerbose("network_addresses", max(0, len(plan.Addresses)-1)))
	case config.ExposureInterfaces:
		fields = append(fields, logger.WithVerbose("interfaces", cfg.Server.Expose.Interfaces), logger.WithVerbose("network_addresses", max(0, len(plan.Addresses)-1)))
	}
	log.Ready("SERVER", "server.ready", "Server ready", fields...)
}

func logEndpointDetails(log *logger.Logger, cfg config.Config) {
	log.Detail("expose", cfg.Server.Expose.Mode)
	if len(cfg.Server.Expose.Interfaces) > 0 {
		log.Detail("interfaces", cfg.Server.Expose.Interfaces)
	}
	plan, err := resolveListenerPlan(cfg.Server.Expose)
	if err != nil {
		log.Detail("network", err.Error())
		return
	}
	for _, address := range plan.Addresses {
		log.Detail(endpointDetailLabel("mcp", address), endpointURL(address.Host, cfg.Server.Port, "/mcp"))
		if cfg.Admin.Enabled {
			log.Detail(endpointDetailLabel("admin", address), endpointURL(address.Host, cfg.Admin.Port, "/"))
		}
	}
	if !cfg.Admin.Enabled {
		log.Detail("admin", "disabled")
	}
}

func endpointDetailLabel(kind string, address mcpnetwork.Address) string {
	if address.Interface != "" {
		return kind + " " + address.Interface
	}
	return kind + " " + address.Scope
}
