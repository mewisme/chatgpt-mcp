package outboundpolicy

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateURLRejectsMetadataAndPrivateByDefault(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{
		"https://169.254.169.254/latest/meta-data/",
		"https://10.0.0.1/mcp",
		"https://192.168.1.1/mcp",
		"https://172.16.0.1/mcp",
		"http://example.com/mcp",
		"https://127.0.0.1/mcp",
		"http://127.0.0.1/mcp",
		"https://user:pass@example.com/mcp",
	} {
		if err := ValidateURL(ctx, raw, Options{}); err == nil {
			t.Fatalf("expected rejection for %s", raw)
		}
	}
}

func TestValidateURLAllowPrivatePermitsLoopback(t *testing.T) {
	ctx := context.Background()
	if err := ValidateURL(ctx, "http://127.0.0.1:9/mcp", Options{AllowPrivate: true}); err != nil {
		t.Fatalf("loopback with allow_private: %v", err)
	}
	if err := ValidateURL(ctx, "https://10.0.0.1/mcp", Options{AllowPrivate: true}); err != nil {
		t.Fatalf("private HTTPS with allow_private: %v", err)
	}
	if err := ValidateURL(ctx, "http://10.0.0.1/mcp", Options{AllowPrivate: true}); err == nil {
		t.Fatal("http to non-loopback private should still be rejected")
	}
}

func TestHTTPClientRejectsRedirectToPrivateHTTP(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.0.0.1/mcp", http.StatusFound)
	}))
	defer source.Close()

	client := NewHTTPClient(Options{AllowPrivate: true})
	request, err := http.NewRequest(http.MethodGet, source.URL+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil {
		t.Fatal("expected redirect to private http host to be rejected")
	}
}

func TestIsPublicIP(t *testing.T) {
	if !IsPublicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("8.8.8.8 should be public")
	}
	if IsPublicIP(net.ParseIP("169.254.169.254")) {
		t.Fatal("metadata IP should not be public")
	}
	if IsPublicIP(net.ParseIP("10.0.0.1")) {
		t.Fatal("RFC1918 should not be public")
	}
}
