package interactive

import (
	"bytes"
	"testing"
)

func TestResolveModeFallsBackOutsideTTY(t *testing.T) {
	var input, output bytes.Buffer
	if interactive, err := ResolveMode(&input, &output, false, false, false); err != nil || interactive {
		t.Fatalf("fallback interactive=%t err=%v", interactive, err)
	}
	if _, err := ResolveMode(&input, &output, true, false, false); err == nil {
		t.Fatal("forced non-TTY interactive mode unexpectedly succeeded")
	}
	if interactive, err := ResolveMode(&input, &output, false, false, true); err != nil || interactive {
		t.Fatalf("json interactive=%t err=%v", interactive, err)
	}
	if _, err := ResolveMode(&input, &output, true, true, false); err == nil {
		t.Fatal("conflicting interactive flags unexpectedly succeeded")
	}
}
