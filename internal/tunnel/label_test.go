package tunnel

import "testing"

func TestDisplayLabelPrefersCachedName(t *testing.T) {
	if got := DisplayLabel("tunnel_a", " Alpha "); got != "Alpha" {
		t.Fatalf("named=%q", got)
	}
	if got := DisplayLabel(" tunnel_a ", "  "); got != "tunnel_a" {
		t.Fatalf("id fallback=%q", got)
	}
	if got := DisplayLabel("  ", "  "); got != "" {
		t.Fatalf("empty=%q", got)
	}
}
