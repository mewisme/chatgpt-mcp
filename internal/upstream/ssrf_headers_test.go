package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeServerRejectsDisallowedURLsAndHeaders(t *testing.T) {
	cases := []Server{
		{ID: "meta", Transport: "http", URL: "https://169.254.169.254/latest/meta-data/"},
		{ID: "rfc1918", Transport: "http", URL: "https://10.0.0.1/mcp"},
		{ID: "plain", Transport: "http", URL: "http://example.com/mcp"},
		{ID: "loop", Transport: "http", URL: "http://127.0.0.1:9/mcp"},
		{ID: "userinfo", Transport: "http", URL: "https://user:pass@example.com/mcp"},
		{ID: "hosthdr", Transport: "http", URL: "https://example.com/mcp", Headers: map[string]string{"Host": "evil.example"}},
		{ID: "crlf", Transport: "http", URL: "https://example.com/mcp", Headers: map[string]string{"X-Test": "a\r\nb"}},
	}
	for _, server := range cases {
		if _, err := NormalizeServer(server); err == nil {
			t.Fatalf("expected rejection for %#v", server)
		}
	}
}

func TestNormalizeServerAllowPrivateNetworkPermitsLoopback(t *testing.T) {
	value, err := NormalizeServer(Server{
		ID: "local", Transport: "http", URL: "http://127.0.0.1:37421/mcp", AllowPrivateNetwork: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !value.AllowPrivateNetwork {
		t.Fatal("allow_private_network was cleared")
	}
}

func TestNormalizeServerRejectsHostAndCRLFHeaders(t *testing.T) {
	if _, err := NormalizeServer(Server{
		ID: "demo", Transport: "http", URL: "https://example.com/mcp",
		Headers: map[string]string{"Host": "evil"},
	}); err == nil || !strings.Contains(err.Error(), "Host") {
		t.Fatalf("host header error=%v", err)
	}
	if _, err := NormalizeServer(Server{
		ID: "demo", Transport: "http", URL: "https://example.com/mcp",
		Headers: map[string]string{"X-Test": "ok\ninjected"},
	}); err == nil || !strings.Contains(err.Error(), "CR/LF") {
		t.Fatalf("crlf header error=%v", err)
	}
}

func TestNativeClientRejectsPrivateUpstreamByDefault(t *testing.T) {
	client := NewNativeClient()
	err := client.Connect(context.Background(), Server{
		ID: "meta", Enabled: true, Transport: "http", URL: "https://169.254.169.254/",
	})
	if err == nil {
		t.Fatal("metadata URL was accepted")
	}
}

func TestNativeClientAllowPrivateFollowsLoopback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}`))
	}))
	defer server.Close()

	client := NewNativeClient()
	if err := client.Connect(context.Background(), Server{
		ID: "local", Enabled: true, Transport: "http", URL: server.URL, Auth: AuthConfig{Type: "none"}, AllowPrivateNetwork: true,
	}); err != nil {
		// negotiate may fail after discover; ensure URL/dial policy allowed the request
		if strings.Contains(err.Error(), "allow_private_network") || strings.Contains(err.Error(), "disallowed") || strings.Contains(err.Error(), "loopback") {
			t.Fatalf("private policy blocked loopback: %v", err)
		}
	}
}

func TestNativeClientRejectsRedirectToPrivateHTTP(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.0.0.1/mcp", http.StatusFound)
	}))
	defer source.Close()

	client := NewNativeClient()
	err := client.Connect(context.Background(), Server{
		ID: "redir", Enabled: true, Transport: "http", URL: source.URL, Auth: AuthConfig{Type: "none"}, AllowPrivateNetwork: true,
	})
	if err == nil {
		t.Fatal("expected redirect to private http to fail")
	}
}
