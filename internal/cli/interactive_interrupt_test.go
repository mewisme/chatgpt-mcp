package cli

import (
	"bytes"
	"testing"
)

func TestInteractiveInterruptKeys(t *testing.T) {
	for _, test := range []struct {
		value byte
		want  string
		ok    bool
	}{{'q', "q", true}, {'Q', "q", true}, {3, "Ctrl+C", true}, {'x', "", false}} {
		got, ok := interactiveInterruptKey(test.value)
		if got != test.want || ok != test.ok {
			t.Fatalf("key %d = (%q, %t), want (%q, %t)", test.value, got, ok, test.want, test.ok)
		}
	}
}

func TestTerminalRawWriterUsesCRLF(t *testing.T) {
	var output bytes.Buffer
	writer := newTerminalRawWriter(&output)
	for _, chunk := range []string{"first\nsecond\r", "\nthird", "\nfourth\n"} {
		if _, err := writer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := output.String(), "first\r\nsecond\r\nthird\r\nfourth\r\n"; got != want {
		t.Fatalf("raw terminal output = %q, want %q", got, want)
	}
}
