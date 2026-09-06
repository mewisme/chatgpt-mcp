package interactive

import (
	"bytes"
	"testing"
)

func TestResolveModeFallsBackOutsideTTY(t *testing.T) {
	var input, output bytes.Buffer
	if interactive, err := ResolveMode(&input, &output, false, false, true, false); err != nil || interactive {
		t.Fatalf("fallback interactive=%t err=%v", interactive, err)
	}
	if _, err := ResolveMode(&input, &output, true, false, false, false); err == nil {
		t.Fatal("forced non-TTY interactive mode unexpectedly succeeded")
	}
	if interactive, err := ResolveMode(&input, &output, false, false, true, true); err != nil || interactive {
		t.Fatalf("json interactive=%t err=%v", interactive, err)
	}
	if _, err := ResolveMode(&input, &output, true, true, true, false); err == nil {
		t.Fatal("conflicting interactive flags unexpectedly succeeded")
	}
}

func TestResolveModePrecedence(t *testing.T) {
	tests := []struct {
		name       string
		force      bool
		disable    bool
		configured bool
		structured bool
		terminal   bool
		want       bool
		wantErr    bool
	}{
		{name: "config enabled tty", configured: true, terminal: true, want: true},
		{name: "config enabled non tty", configured: true},
		{name: "config disabled tty", configured: false, terminal: true},
		{name: "force overrides disabled config", force: true, configured: false, terminal: true, want: true},
		{name: "disable overrides enabled config", disable: true, configured: true, terminal: true},
		{name: "structured overrides force", force: true, configured: true, structured: true, terminal: true},
		{name: "force requires tty", force: true, configured: false, wantErr: true},
		{name: "conflicting flags", force: true, disable: true, configured: true, terminal: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveMode(test.force, test.disable, test.configured, test.structured, test.terminal)
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("mode=%t err=%v, want mode=%t err=%t", got, err, test.want, test.wantErr)
			}
		})
	}
}
