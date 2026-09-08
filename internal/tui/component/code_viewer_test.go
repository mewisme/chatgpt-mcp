package component

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/tui/testutil"
)

func TestCodeViewerPreservesRawContentAndWrapsWithoutHorizontalScroll(t *testing.T) {
	content := "{\n  \"long\": \"" + strings.Repeat("x", 80) + "\"\n}"
	viewer := NewCodeViewer(content)
	viewer.Resize(24, 4)
	if viewer.Content() != content {
		t.Fatalf("content changed: %q", viewer.Content())
	}
	testutil.AssertLinesFit(t, viewer.View(), 24)
	updated, _ := viewer.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	viewer = updated
	if viewer.XOffset() != 0 {
		t.Fatalf("horizontal offset=%d want 0", viewer.XOffset())
	}
	viewer.Resize(12, 4)
	testutil.AssertLinesFit(t, viewer.View(), 12)
}

func TestCodeViewerMouseWheelScrollsVertically(t *testing.T) {
	viewer := NewCodeViewer(strings.Repeat("line\n", 20))
	viewer.Resize(40, 4)
	message := viewer.MouseTargets(0, 0, 0)[0].Handle(MouseEvent{Button: tea.MouseWheelDown})
	updated, _ := viewer.Update(message)
	viewer = updated
	if viewer.YOffset() == 0 {
		t.Fatal("mouse wheel did not scroll vertically")
	}
}
