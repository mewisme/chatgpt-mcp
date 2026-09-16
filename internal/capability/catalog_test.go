package capability

import (
	"strings"
	"testing"
)

func TestCatalogHasUniqueIDsAndPaths(t *testing.T) {
	ids, paths := map[ID]bool{}, map[string]ID{}
	for _, spec := range All() {
		if spec.ID == "" || NormalizePath(spec.CanonicalPath) == "" {
			t.Fatalf("invalid capability: %#v", spec)
		}
		if ids[spec.ID] {
			t.Fatalf("duplicate capability id: %s", spec.ID)
		}
		ids[spec.ID] = true
		for _, path := range append([]string{spec.CanonicalPath}, spec.PublicPaths...) {
			path = NormalizePath(path)
			if previous, ok := paths[path]; ok && previous != spec.ID {
				t.Fatalf("path %q maps to both %s and %s", path, previous, spec.ID)
			}
			paths[path] = spec.ID
			if got, ok := ForPath(path); !ok || got != spec.ID {
				t.Fatalf("ForPath(%q)=%q,%t want %q", path, got, ok, spec.ID)
			}
		}
	}
}

func TestCatalogHasNoOAuthCapabilities(t *testing.T) {
	for _, spec := range All() {
		blob := strings.ToLower(string(spec.ID) + " " + spec.CanonicalPath + " " + strings.Join(spec.PublicPaths, " "))
		if strings.Contains(blob, "oauth") {
			t.Fatalf("oauth capability leftover: %#v", spec)
		}
	}
}

func TestAuthCapabilitiesStayDistinct(t *testing.T) {
	want := map[string]ID{
		"auth mcp rotate":    AuthMCPRotate,
		"auth mcp create":    AuthMCPRotate,
		"auth mcp show":      AuthMCPShow,
		"auth mcp copy":      AuthMCPCopy,
		"auth mcp enable":    AuthMCPEnable,
		"auth mcp disable":   AuthMCPDisable,
		"auth admin create":  AuthAdminRotate,
		"auth admin enable":  AuthAdminEnable,
		"auth admin disable": AuthAdminDisable,
		"auth status":        AuthStatus,
		"auth mcp status":    AuthStatus,
	}
	for path, id := range want {
		got, ok := ForPath(path)
		if !ok || got != id {
			t.Fatalf("ForPath(%q)=%q,%t want %q", path, got, ok, id)
		}
	}
	for _, path := range []string{"auth mcp rotate", "auth mcp show", "auth mcp copy", "auth mcp enable", "auth mcp disable"} {
		got, _ := ForPath(path)
		if got == AuthStatus || got == AuthAdminRotate || strings.HasPrefix(string(got), "auth.admin.") || strings.HasPrefix(string(got), "tunnel.") {
			t.Fatalf("Direct MCP HTTP path %q collapsed to %q", path, got)
		}
	}
	if got, _ := ForPath("auth admin create"); got == AuthMCPRotate || got == AuthStatus || strings.HasPrefix(string(got), "auth.mcp.") || strings.HasPrefix(string(got), "tunnel.") {
		t.Fatalf("admin rotate collapsed to %q", got)
	}
}
