package cli

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
)

func TestWaitRuntimeHTTPReadyRequiresMCPAndAdminListeners(t *testing.T) {
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mcp.Close()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer admin.Close()
	cfg := config.Default()
	cfg.Server.Port = testServerPort(t, mcp.Listener.Addr())
	cfg.Admin.Enabled = true
	cfg.Admin.Port = testServerPort(t, admin.Listener.Addr())
	if err := waitRuntimeHTTPReady(context.Background(), cfg, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWaitRuntimeHTTPReadyRejectsMissingListener(t *testing.T) {
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer mcp.Close()
	missing, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	missingPort := testServerPort(t, missing.Addr())
	_ = missing.Close()
	cfg := config.Default()
	cfg.Server.Port = testServerPort(t, mcp.Listener.Addr())
	cfg.Admin.Enabled = true
	cfg.Admin.Port = missingPort
	err = waitRuntimeHTTPReady(context.Background(), cfg, 100*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "server listeners did not become ready") {
		t.Fatalf("error = %v", err)
	}
}

func testServerPort(t *testing.T, address net.Addr) int {
	t.Helper()
	_, portText, err := net.SplitHostPort(address.String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return port
}
