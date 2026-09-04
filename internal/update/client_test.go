package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientLatest(t *testing.T) {
	asset, err := CurrentAssetName("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	var gotPath, gotAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v1.2.3","draft":false,"assets":[{"name":%q,"browser_download_url":"https://example.test/archive"},{"name":"checksums.txt","browser_download_url":"https://example.test/checksums"}]}`, asset)
	}))
	defer server.Close()

	release, err := (Client{BaseURL: server.URL, HTTPClient: server.Client(), UserAgent: "chatgpt-mcp/test"}).Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/repos/mewisme/chatgpt-mcp/releases/latest" {
		t.Fatalf("request path = %q", gotPath)
	}
	if gotAgent != "chatgpt-mcp/test" {
		t.Fatalf("user agent = %q", gotAgent)
	}
	if release.Version != "v1.2.3" || release.ArchiveName != asset || release.ArchiveURL != "https://example.test/archive" || release.ChecksumURL != "https://example.test/checksums" {
		t.Fatalf("release = %+v", release)
	}
}

func TestClientLatestRequiresExpectedAssets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v1.2.3","assets":[]}`)
	}))
	defer server.Close()
	if _, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).Latest(context.Background()); err == nil {
		t.Fatal("missing release assets were accepted")
	}
}

func TestClientLatestRejectsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()
	if _, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).Latest(context.Background()); err == nil {
		t.Fatal("HTTP failure was accepted")
	}
}
