//go:build !windows

package tools

import "testing"

func TestDeleteDirectoryFallbackQuotesUnixPath(t *testing.T) {
	got := deleteDirectoryFallback("/tmp/a b/'quoted'")
	want := "rm -rf -- '/tmp/a b/'\"'\"'quoted'\"'\"''"
	if got != want {
		t.Fatalf("fallback=%q want=%q", got, want)
	}
}
