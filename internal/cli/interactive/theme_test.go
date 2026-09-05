package interactive

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCharmThemeSemanticStylesDoNotInjectComponentGlyphs(t *testing.T) {
	SetDarkBackground(true)
	for name, value := range map[string]string{
		"accent":  ToneText("> ", ToneAccent),
		"success": ToneText("[x]", ToneSuccess),
		"danger":  ToneText("error", ToneDanger),
	} {
		want := map[string]string{"accent": "> ", "success": "[x]", "danger": "error"}[name]
		if got := ansi.Strip(value); got != want {
			t.Fatalf("%s rendered %q, want %q", name, got, want)
		}
	}
}
