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

func TestUniqueLabelsDisambiguateDuplicateNames(t *testing.T) {
	ids := []string{"tunnel_aaaaaaaaaaaaaaaaaaaaaaaac3330bcd", "tunnel_bbbbbbbbbbbbbbbbbbbbbbb3ce094ac", "tunnel_anon"}
	names := []string{"Production", "Production", ""}
	got := UniqueLabels(ids, names)
	want := []string{"Production · c3330bcd", "Production · 3ce094ac", "tunnel_anon"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%q want=%q", got, want)
		}
	}
	if UniqueLabel("tunnel_only", "Staging", false) != "Staging" {
		t.Fatalf("unique named=%q", UniqueLabel("tunnel_only", "Staging", false))
	}
}
