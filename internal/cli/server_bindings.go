package cli

import (
	"errors"
	"net"
	"net/http"
	"slices"

	"go.mewis.me/chatgpt-mcp/internal/app"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

type httpBindings struct {
	cfg            config.Config
	plan           listenerPlan
	mcpListeners   []net.Listener
	adminListeners []net.Listener
	servers        []*http.Server
}

func openHTTPBindings(cfg config.Config, plan listenerPlan) (*httpBindings, error) {
	mcpListeners, err := listenOnHosts(plan.Hosts, cfg.Server.Port)
	if err != nil {
		return nil, err
	}
	bindings := &httpBindings{cfg: cfg, plan: plan, mcpListeners: mcpListeners}
	if cfg.Admin.Enabled {
		bindings.adminListeners, err = listenOnHosts(plan.Hosts, cfg.Admin.Port)
		if err != nil {
			closeListeners(mcpListeners)
			return nil, err
		}
	}
	return bindings, nil
}

func (b *httpBindings) Start(runtime *app.App, errCh chan<- error) {
	if b == nil {
		return
	}
	b.servers = make([]*http.Server, 0, len(b.mcpListeners)+len(b.adminListeners))
	for _, listener := range b.mcpListeners {
		server := newHTTPServer(runtime.MCPHandler())
		b.servers = append(b.servers, server)
		go serveHTTP(server, listener, errCh)
	}
	for _, listener := range b.adminListeners {
		server := newHTTPServer(runtime.AdminHandler())
		b.servers = append(b.servers, server)
		go serveHTTP(server, listener, errCh)
	}
}

func (b *httpBindings) Shutdown() error {
	if b == nil {
		return nil
	}
	err := shutdownServers(b.servers)
	closeListeners(b.mcpListeners)
	closeListeners(b.adminListeners)
	b.servers = nil
	b.mcpListeners = nil
	b.adminListeners = nil
	return err
}

func (b *httpBindings) CloseUnstarted() {
	if b == nil {
		return
	}
	closeListeners(b.mcpListeners)
	closeListeners(b.adminListeners)
	b.mcpListeners = nil
	b.adminListeners = nil
}

func networkConfigEqual(left, right config.Config) bool {
	return left.Server.Port == right.Server.Port && config.ExposureEqual(left.Server.Expose, right.Server.Expose) && left.Admin == right.Admin
}

func listenerPlanEqual(left, right listenerPlan) bool { return slices.Equal(left.Hosts, right.Hosts) }

func restoreHTTPBindings(runtime *app.App, cfg config.Config, plan listenerPlan, errCh chan<- error) (*httpBindings, error) {
	bindings, err := openHTTPBindings(cfg, plan)
	if err != nil {
		return nil, err
	}
	bindings.Start(runtime, errCh)
	return bindings, nil
}

func serveHTTP(server *http.Server, listener net.Listener, errCh chan<- error) {
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errCh <- err
	}
}
