package cli

import "testing"

func TestInteractiveInterruptKeys(t *testing.T) {
	for _, test := range []struct {
		value byte
		want  string
		ok    bool
	}{{'q', "q", true}, {'Q', "q", true}, {3, "Ctrl+C", true}, {'x', "", false}} {
		got, ok := interactiveInterruptKey(test.value)
		if got != test.want || ok != test.ok {
			t.Fatalf("key %d = (%q, %t), want (%q, %t)", test.value, got, ok, test.want, test.ok)
		}
	}
}
