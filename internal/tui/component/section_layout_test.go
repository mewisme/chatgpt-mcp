package component

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestSectionLayoutUsesAllRowsBeforeFooter(t *testing.T) {
	layout := NewSectionLayout("Logs", "live", "", 80, 20, 1)
	prefixHeight := lipgloss.Height(layout.Header)
	if layout.BodyY != prefixHeight {
		t.Fatalf("body y=%d want=%d", layout.BodyY, prefixHeight)
	}
	if layout.BodyHeight != 20-prefixHeight-1 {
		t.Fatalf("body height=%d want=%d", layout.BodyHeight, 20-prefixHeight-1)
	}
}

func TestSectionLayoutCountsFeedbackRowsExactly(t *testing.T) {
	layout := NewSectionLayout("Logs", "live", "warning\nretrying", 80, 20, 1)
	prefixHeight := lipgloss.Height(layout.Header)
	if layout.BodyY != prefixHeight || layout.BodyHeight != 20-prefixHeight-1 {
		t.Fatalf("layout y=%d height=%d prefix=%d", layout.BodyY, layout.BodyHeight, prefixHeight)
	}
}
