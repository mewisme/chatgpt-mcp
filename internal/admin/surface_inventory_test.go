package admin

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestSurfaceInventoryAdminAPIRoutes(t *testing.T) {
	routes := collectAdminAPIRoutes(t)
	if len(routes) < 10 {
		t.Fatalf("too few admin routes: %v", routes)
	}
	for _, want := range []string{"/api/health", "/api/plugins", "/api/tunnels", "/api/requests", "/api/config"} {
		if !containsString(routes, want) {
			t.Fatalf("missing admin route %s in %v", want, routes)
		}
	}
	for _, route := range routes {
		if strings.Contains(strings.ToLower(route), "oauth") {
			t.Fatalf("stale oauth route %s", route)
		}
	}
	ui := collectAdminUIRoutes(t)
	for _, want := range []string{"overview", "tunnel", "workspaces", "settings"} {
		if !containsString(ui, want) {
			t.Fatalf("missing admin ui route %s in %v", want, ui)
		}
	}
	if containsString(ui, "oauth") {
		t.Fatal("stale oauth admin ui route")
	}
	testutil.WriteLocalJSON(t, "surface-parity-admin.json", map[string]any{"api_routes": routes, "ui_routes": ui})
}

func collectAdminAPIRoutes(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`mux\.HandleFunc\("([^"]+)"`).FindAllStringSubmatch(string(data), -1)
	seen := map[string]bool{}
	out := []string{}
	for _, match := range matches {
		route := match[1]
		if seen[route] {
			continue
		}
		seen[route] = true
		out = append(out, route)
	}
	sort.Strings(out)
	return out
}

func collectAdminUIRoutes(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testutil.RepoRoot(t), "plugins", "admin-ui", "src", "router.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`path: "([^"]+)"`).FindAllStringSubmatch(string(data), -1)
	out := []string{}
	for _, match := range matches {
		out = append(out, match[1])
	}
	sort.Strings(out)
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
