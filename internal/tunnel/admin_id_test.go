package tunnel

import (
	"strings"
	"testing"
)

func TestValidateAdminProfileID(t *testing.T) {
	for _, id := range []string{"personal", "personal-1", "personal.dev", "personal_test", "P", " default "} {
		if err := ValidateAdminProfileID(id); err != nil {
			t.Fatalf("%q: %v", id, err)
		}
	}
	for _, id := range []string{"", "   ", "my profile", "personal/test", `personal\test`, "personal\nid", "-lead", "trail.", strings.Repeat("a", 65)} {
		if err := ValidateAdminProfileID(id); err == nil {
			t.Fatalf("%q accepted", id)
		}
	}
}

func TestCollectionConfigValidateAllowsLegacyProfileIDs(t *testing.T) {
	cfg := CollectionConfig{Admins: []AdminConfig{{ID: "my profile"}}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateControlPlaneBaseURL(t *testing.T) {
	if err := ValidateControlPlaneBaseURL(""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateControlPlaneBaseURL("https://chatgpt.com/backend-api/wham"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateControlPlaneBaseURL("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateControlPlaneBaseURL("http://example.test"); err == nil {
		t.Fatal("expected non-loopback http to fail")
	}
	if err := ValidateControlPlaneBaseURL("not a url"); err == nil {
		t.Fatal("expected invalid url to fail")
	}
}
