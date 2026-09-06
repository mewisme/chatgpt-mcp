package application

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
)

func TestRuntimeStatusReturnsStoppedWithoutControlState(t *testing.T) {
	setupLogsRoot(t)
	status, running, err := RuntimeStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if running || status.PID != 0 {
		t.Fatalf("status=%#v running=%t", status, running)
	}
}

func TestRuntimeStatusUsesAuthenticatedControlEndpoint(t *testing.T) {
	root := setupLogsRoot(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("request path=%q auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(runtimecontrol.RuntimeStatus{PID: os.Getpid(), RunID: "run_test", Managed: true, ServiceID: "service", ServiceScope: "user"})
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	writeRuntimeState(t, root, runtimecontrol.State{PID: os.Getpid(), Address: parsed.Host, Token: "token", ConfigRoot: root})
	status, running, err := RuntimeStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !running || status.RunID != "run_test" || !status.Managed || status.ServiceScope != "user" {
		t.Fatalf("status=%#v running=%t", status, running)
	}
}

func TestManagedRuntimeSystemActionReturnsExternalElevationWorkflow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("system scope is unsupported on Windows")
	}
	setupLogsRoot(t)
	previous := detectServiceScope
	detectServiceScope = func() managed.Scope { return managed.ScopeUser }
	t.Cleanup(func() { detectServiceScope = previous })
	result, err := ManagedRuntimeAction(t.Context(), "restart", managed.ScopeSystem)
	if err != nil {
		t.Fatal(err)
	}
	if result.External == nil || !strings.Contains(result.External.Command, "restart --system") || !strings.Contains(strings.ToLower(result.External.Reason), "elevation") {
		t.Fatalf("external=%#v", result.External)
	}
}

func TestLoadAboutReportsMachineAndServerUptime(t *testing.T) {
	root := setupLogsRoot(t)
	previous := machineUptime
	machineUptime = func() (time.Duration, error) { return 3*time.Hour + 4*time.Minute, nil }
	t.Cleanup(func() { machineUptime = previous })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(runtimecontrol.RuntimeStatus{PID: os.Getpid(), StartedAt: time.Now().Add(-2 * time.Minute)})
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	writeRuntimeState(t, root, runtimecontrol.State{PID: os.Getpid(), Address: parsed.Host, Token: "token", ConfigRoot: root})
	info, err := LoadAbout(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !info.MachineUptimeOK || info.MachineUptime != 3*time.Hour+4*time.Minute || !info.RuntimeRunning || info.ServerUptime < time.Minute {
		t.Fatalf("about=%#v", info)
	}
}
