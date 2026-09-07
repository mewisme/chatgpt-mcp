package component

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestToastDialogHasSingleCloseAction(t *testing.T) {
	dialog := NewToastDialog("Updated", "Workspace saved", ToneSuccess)
	plain := ansi.Strip(dialog.View())
	for _, want := range []string{"✓ Updated", "Workspace saved", "Close"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("toast dialog missing %q: %q", want, plain)
		}
	}
	if strings.Count(plain, "Close") != 1 {
		t.Fatalf("toast dialog close actions=%d: %q", strings.Count(plain, "Close"), plain)
	}
}

func TestToastDialogTitleOnlyDoesNotDuplicateTitle(t *testing.T) {
	plain := ansi.Strip(NewToastDialog("Saved", "", ToneSuccess).View())
	if strings.Count(plain, "Saved") != 1 || strings.Count(plain, "Close") != 1 {
		t.Fatalf("title-only toast duplicated content: %q", plain)
	}
}
