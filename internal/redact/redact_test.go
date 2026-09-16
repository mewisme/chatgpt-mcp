package redact

import (
	"fmt"
	"strings"
	"testing"
)

func TestTextRedactsSecretsAndSignedURLs(t *testing.T) {
	input := `request failed https://user:pass@example.test/path?token=query-secret&safe=1 Authorization: Bearer bearer-secret api_key=plain-secret access_token="json-secret" mcp_abcdefghijklmnopqrstuvwxyz012345`
	got := Text(input)
	for _, secret := range []string{"user:pass@", "query-secret", "bearer-secret", "plain-secret", "json-secret", "mcp_abcdefghijklmnopqrstuvwxyz012345"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted text leaked %q: %q", secret, got)
		}
	}
	if !strings.Contains(got, "safe=1") || !strings.Contains(got, "<redacted>") {
		t.Fatalf("redacted text lost safe context: %q", got)
	}
}

func TestValueRecursivelyRedactsNestedSecrets(t *testing.T) {
	value := Value("payload", map[string]any{
		"api_key": "field-secret",
		"safe":    "ok",
		"nested":  map[string]string{"client_secret": "nested-secret", "route_kind": "mcp_channel"},
		"items":   []any{"Bearer bearer-secret", map[string]any{"token": "hidden-token"}},
	})
	text := fmt.Sprint(value)
	for _, secret := range []string{"field-secret", "nested-secret", "bearer-secret", "hidden-token"} {
		if strings.Contains(text, secret) {
			t.Fatalf("nested value leaked %q: %q", secret, text)
		}
	}
	for _, safe := range []string{"ok", "mcp_channel"} {
		if !strings.Contains(text, safe) {
			t.Fatalf("nested value lost %q: %q", safe, text)
		}
	}
}

func TestURLStripsUserInfoAndSensitiveQueryOnly(t *testing.T) {
	got := URL("https://user:pass@example.test/plugin?X-Amz-Signature=signed-secret&channel=stable&token=query-secret")
	for _, secret := range []string{"user", "pass", "signed-secret", "query-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitized URL leaked %q: %q", secret, got)
		}
	}
	if !strings.Contains(got, "channel=stable") {
		t.Fatalf("sanitized URL lost safe query: %q", got)
	}
}

func TestTextPreservesAlreadyRedactedMarkersAndNonSecretMCPValues(t *testing.T) {
	got := Text("api_key=[redacted] route_kind=mcp_channel runtime_unavailable")
	if got != "api_key=[redacted] route_kind=mcp_channel runtime_unavailable" {
		t.Fatalf("already-redacted text changed: %q", got)
	}
}
