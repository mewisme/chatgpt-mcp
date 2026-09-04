package interactive

import (
	"bytes"
	"testing"
)

func TestCursorWrapsAndClamps(t *testing.T) {
	cursor := Cursor{}
	cursor.Move(-1, 3)
	if cursor.Index != 2 {
		t.Fatalf("wrapped cursor=%d", cursor.Index)
	}
	cursor.Move(1, 3)
	if cursor.Index != 0 {
		t.Fatalf("forward cursor=%d", cursor.Index)
	}
	cursor.Index = 8
	cursor.Clamp(2)
	if cursor.Index != 1 {
		t.Fatalf("clamped cursor=%d", cursor.Index)
	}
}

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
