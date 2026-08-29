package cli

import (
	"net"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
)

const (
	loopbackHost = "127.0.0.1"
	exposedHost  = "0.0.0.0"
)

type runtimeAddress struct {
	Host      string
	Interface string
	Scope     string
}

type networkCandidate struct {
	IP        net.IP
	Interface string
	Flags     net.Flags
}

func addExposeFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("expose", false, "expose MCP and admin listeners on all active network interfaces for this run")
}

func applyExposeOverride(cmd *cobra.Command, cfg *config.Config) error {
	flag := cmd.Flags().Lookup("expose")
	if flag == nil || !flag.Changed {
		return nil
	}
	expose, err := cmd.Flags().GetBool("expose")
	if err != nil {
		return err
	}
	cfg.Server.Expose = expose
	return nil
}

func serverBindHost(expose bool) string {
	if expose {
		return exposedHost
	}
	return loopbackHost
}

func serverBindAddress(port int, expose bool) string {
	return net.JoinHostPort(serverBindHost(expose), strconv.Itoa(port))
}

func endpointURL(host string, port int, endpointPath string) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + endpointPath
}

func runtimeAddresses(expose bool) []runtimeAddress {
	addresses := []runtimeAddress{{Host: loopbackHost, Scope: "local"}}
	if !expose {
		return addresses
	}
	return append(addresses, normalizeNetworkAddresses(discoverNetworkCandidates())...)
}

func discoverNetworkCandidates() []networkCandidate {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	candidates := make([]networkCandidate, 0)
	for _, iface := range interfaces {
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip != nil {
				candidates = append(candidates, networkCandidate{IP: ip, Interface: iface.Name, Flags: iface.Flags})
			}
		}
	}
	return candidates
}

func normalizeNetworkAddresses(candidates []networkCandidate) []runtimeAddress {
	addresses := make([]runtimeAddress, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Flags&net.FlagUp == 0 || candidate.Flags&net.FlagLoopback != 0 {
			continue
		}
		ip := candidate.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || !ip.IsGlobalUnicast() {
			continue
		}
		scope := "network"
		if ip.IsPrivate() {
			scope = "lan"
		}
		addresses = append(addresses, runtimeAddress{Host: ip.String(), Interface: candidate.Interface, Scope: scope})
	}
	sort.Slice(addresses, func(i, j int) bool {
		if (addresses[i].Scope == "lan") != (addresses[j].Scope == "lan") {
			return addresses[i].Scope == "lan"
		}
		if addresses[i].Host != addresses[j].Host {
			return addresses[i].Host < addresses[j].Host
		}
		return addresses[i].Interface < addresses[j].Interface
	})
	unique := addresses[:0]
	seen := map[string]struct{}{}
	for _, address := range addresses {
		if _, ok := seen[address.Host]; ok {
			continue
		}
		seen[address.Host] = struct{}{}
		unique = append(unique, address)
	}
	return unique
}

func logReadyEndpoints(log *logger.Logger, cfg config.Config) {
	addresses := runtimeAddresses(cfg.Server.Expose)
	if cfg.Server.Expose {
		log.Info("SERVER", "network exposure enabled", "bind", exposedHost, "addresses", len(addresses)-1)
		if len(addresses) == 1 {
			log.Warn("SERVER", "no active non-loopback IPv4 interfaces detected", "bind", exposedHost)
		}
	}
	for _, address := range addresses {
		fields := endpointFields(address, endpointURL(address.Host, cfg.Server.Port, "/mcp"))
		log.Info("MCP", "endpoint ready", fields...)
		if cfg.Admin.Enabled {
			fields = endpointFields(address, endpointURL(address.Host, cfg.Admin.Port, "/"))
			log.Info("ADMIN", "dashboard ready", fields...)
		}
	}
}

func logEndpointDetails(log *logger.Logger, cfg config.Config) {
	log.Detail("expose", cfg.Server.Expose)
	for _, address := range runtimeAddresses(cfg.Server.Expose) {
		log.Detail(endpointDetailLabel("mcp", address), endpointURL(address.Host, cfg.Server.Port, "/mcp"))
		if cfg.Admin.Enabled {
			log.Detail(endpointDetailLabel("admin", address), endpointURL(address.Host, cfg.Admin.Port, "/"))
		}
	}
	if !cfg.Admin.Enabled {
		log.Detail("admin", "disabled")
	}
}

func endpointFields(address runtimeAddress, url string) []any {
	fields := []any{"url", url, "scope", address.Scope}
	if address.Interface != "" {
		fields = append(fields, "interface", address.Interface)
	}
	return fields
}

func endpointDetailLabel(kind string, address runtimeAddress) string {
	if address.Interface != "" {
		return kind + " " + address.Interface
	}
	return kind + " " + address.Scope
}
