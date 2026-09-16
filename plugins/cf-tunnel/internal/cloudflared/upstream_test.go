package cloudflared

import (
	"os"
	"strings"
	"testing"
)

func TestUpstreamPinMatchesReportedVersion(t *testing.T) {
	body, err := os.ReadFile("UPSTREAM.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "`"+reportedVersion+"`") {
		t.Fatalf("UPSTREAM.md missing tag %s", reportedVersion)
	}
	if !strings.Contains(text, "f11dea9cb7079e90a982c1a2d5548ab40847fdcf") {
		t.Fatal("UPSTREAM.md missing pinned commit")
	}
}
