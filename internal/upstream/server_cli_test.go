package upstream

import "testing"

func TestNormalizeServerPreservesExposeWhenDisabled(t *testing.T) {
	value, err := NormalizeServer(Server{ID: "demo", Transport: "http", URL: "http://example.test", Enabled: false, Expose: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if value.Expose != "all" {
		t.Fatalf("expose = %q, want all", value.Expose)
	}
}
