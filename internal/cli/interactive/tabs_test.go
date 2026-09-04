package interactive

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTabsMoveWrapAndRender(t *testing.T) {
	if got := MoveTab(0, 3, -1); got != 2 {
		t.Fatalf("previous wrap=%d", got)
	}
	if got := MoveTab(2, 3, 1); got != 0 {
		t.Fatalf("next wrap=%d", got)
	}
	if delta, ok := TabDelta(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight})); !ok || delta != 1 {
		t.Fatalf("right delta=%d ok=%t", delta, ok)
	}
	view := Tabs([]string{"Overview", "Arguments", "Guard"}, 1)
	for _, label := range []string{"Overview", "Arguments", "Guard"} {
		if !strings.Contains(view, label) {
			t.Fatalf("tabs view missing %q: %q", label, view)
		}
	}
}
