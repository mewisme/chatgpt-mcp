package upstream

import (
	"reflect"
	"testing"
)

func TestNormalizeServerPreservesExposeWhenDisabled(t *testing.T) {
	value, err := NormalizeServer(Server{ID: "demo", Transport: "http", URL: "http://example.test", Enabled: false, Expose: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if value.Expose != "all" {
		t.Fatalf("expose = %q, want all", value.Expose)
	}
}

func TestParseAssignmentsAndRedactServer(t *testing.T) {
	values, err := ParseAssignments([]string{" A =one", "B=two=three"}, "env")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"A": "one", "B": "two=three"}; !reflect.DeepEqual(values, want) {
		t.Fatalf("assignments=%#v want %#v", values, want)
	}
	if _, err := ParseAssignments([]string{"missing"}, "header"); err == nil || err.Error() != "header must use KEY=VALUE: missing" {
		t.Fatalf("invalid assignment error=%v", err)
	}

	original := Server{
		Headers: map[string]string{"Authorization": "Bearer secret", "X-Mode": "safe"},
		Env:     map[string]string{"API_TOKEN": "secret-env", "MODE": "safe-env"},
	}
	redacted := RedactServer(original)
	if redacted.Headers["Authorization"] != "<redacted>" || redacted.Env["API_TOKEN"] != "<redacted>" {
		t.Fatalf("redacted=%#v", redacted)
	}
	if redacted.Headers["X-Mode"] != "safe" || redacted.Env["MODE"] != "safe-env" {
		t.Fatalf("non-sensitive values changed: %#v", redacted)
	}
	if original.Headers["Authorization"] != "Bearer secret" || original.Env["API_TOKEN"] != "secret-env" {
		t.Fatalf("redaction mutated original: %#v", original)
	}
}
