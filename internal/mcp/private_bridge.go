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

	"go.mewis.me/chatgpt-mcp/internal/auth"
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
	streamable := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return adapter.Server }, &sdkmcp.StreamableHTTPOptions{SessionTimeout: 30 * time.Minute, PropagateRequestCancellation: true})
	mux := http.NewServeMux()
	mux.Handle("/mcp", streamable)
	return mux, nil
}

func StartPrivateBridge(runtime *tools.Runtime) (*PrivateBridge, error) {
	handler, err := NewPrivateHTTPHandler(runtime)
	if err != nil {
		return nil, err
	}
	token := auth.GenerateToken("cgm_plugin")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: auth.Middleware(token, loopbackOnly(handler)), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(ln) }()
	return &PrivateBridge{URL: "http://" + ln.Addr().String() + "/mcp", Token: token, listener: ln, server: server}, nil
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

func BearerHTTPClient(token string) *http.Client {
	return &http.Client{Transport: bearerTransport{base: http.DefaultTransport, token: strings.TrimSpace(token)}}
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.base == nil {
		t.base = http.DefaultTransport
	}
	req = req.Clone(req.Context())
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.base.RoundTrip(req)
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
