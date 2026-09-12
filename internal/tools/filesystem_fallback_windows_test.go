//go:build windows

package tools

import "testing"

func TestDeleteDirectoryFallbackQuotesWindowsPath(t *testing.T) {
	got := deleteDirectoryFallback(`C:\work\a b\'quoted'`)
	want := `Remove-Item -LiteralPath 'C:\work\a b\''quoted''' -Recurse -Force`
	if got != want {
		t.Fatalf("fallback=%q want=%q", got, want)
	}
}
