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

func TestAdminAPIClientMethodsHaveUICallersOrExemption(t *testing.T) {
	root := filepath.Join(testutil.RepoRoot(t), "plugins", "admin-ui", "src")
	methods := collectAdminAPIClientMethods(t, filepath.Join(root, "lib", "api.ts"))
	used := map[string]bool{}
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasSuffix(path, "lib/api.ts") {
			return err
		}
		if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, method := range methods {
			if regexp.MustCompile(`adminApi\.` + regexp.QuoteMeta(method) + `\b`).MatchString(text) {
				used[method] = true
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	unused := []string{}
	for _, method := range methods {
		if used[method] {
			continue
		}
		if reason := adminAPIClientExemptions[method]; reason == "" {
			unused = append(unused, method)
			continue
		}
	}
	if len(unused) > 0 {
		t.Fatalf("adminApi methods have no UI caller and no exemption:\n  %s", strings.Join(unused, "\n  "))
	}
	for method, reason := range adminAPIClientExemptions {
		if strings.TrimSpace(reason) == "" {
			t.Fatalf("exemption %s has no reason", method)
		}
		if !containsString(methods, method) {
			t.Fatalf("stale adminApi exemption %s", method)
		}
		if used[method] {
			t.Fatalf("adminApi exemption %s is used by UI; remove it", method)
		}
	}
}

var adminAPIClientExemptions = map[string]string{
	"localTunnel":                     "detail GET is unused; the tunnel page lists and mutates through collection helpers",
	"tunnelAdminProfile":              "detail GET is unused; the tunnel page lists and mutates through collection helpers",
	"managedTunnelByProfile":          "managed-tunnel get-by-id is unused; collection list is the UI path",
	"upstreamServer":                  "Servers page uses the collection plus status/tools helpers, not the single-server GET",
	"workspaceContainer":              "detail GET is unused; container pages use the collection helpers",
	"workspaceContainersForWorkspace": "Workspace container membership uses add/remove collection endpoints, not the per-workspace GET helper",
}

func collectAdminAPIClientMethods(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`(?m)^\s{2}([A-Za-z][A-Za-z0-9]*)\s*:`).FindAllStringSubmatch(string(data), -1)
	out := []string{}
	seen := map[string]bool{}
	inClient := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "export const adminApi") {
			inClient = true
			continue
		}
		if inClient && strings.HasPrefix(line, "}") {
			break
		}
		if !inClient {
			continue
		}
		match := regexp.MustCompile(`^\s{2}([A-Za-z][A-Za-z0-9]*)\s*:`).FindStringSubmatch(line)
		if match == nil {
			continue
		}
		name := match[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		t.Fatalf("no adminApi methods parsed from %s matches=%d", path, len(matches))
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
