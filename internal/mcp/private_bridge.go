package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"go.mewis.me/chatgpt-mcp/internal/tools"
)

type PrivateBridge struct {
	URL      string
	Token    string
	listener net.Listener
	server   *http.Server
}

func NewPrivateHTTPHandler(toolRuntime *tools.Runtime) (http.Handler, error) {
	adapter, err := NewSDKServerWithSession(toolRuntime, "tunnel", "", "")
	if err != nil {
		return nil, err
	}
	adapter.PreferSessionMeta = true
	streamable := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return adapter.Server }, &sdkmcp.StreamableHTTPOptions{Stateless: true, SessionTimeout: 30 * time.Minute, PropagateRequestCancellation: true})
	mux := http.NewServeMux()
	mux.Handle("/mcp", streamable)
	return mux, nil
}

func StartPrivateBridge(runtime *tools.Runtime) (*PrivateBridge, error) {
	handler, err := NewPrivateHTTPHandler(runtime)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: loopbackOnly(streamableTunnelCompat(handler)), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(ln) }()
	return &PrivateBridge{URL: "http://" + ln.Addr().String() + "/mcp", listener: ln, server: server}, nil
}

func (b *PrivateBridge) Close() error {
	if b == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var err error
	if b.server != nil {
		err = b.server.Shutdown(ctx)
	}
	if b.listener != nil {
		err = errors.Join(err, b.listener.Close())
	}
	return err
}

func streamableTunnelCompat(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Accept", "application/json, text/event-stream")
		if r.Method == http.MethodPost {
			if ct := r.Header.Get("Content-Type"); ct == "" || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ct)), "application/json") {
				r.Header.Set("Content-Type", "application/json")
			}
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.RemoteAddr
		if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			host = h
		}
		ip := net.ParseIP(strings.TrimSpace(host))
		if ip == nil || !ip.IsLoopback() {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func ValidateBridgeURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" {
		return fmt.Errorf("private MCP bridge URL is invalid")
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	if strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback()) {
		return nil
	}
	return fmt.Errorf("private MCP bridge must bind to loopback")
}
