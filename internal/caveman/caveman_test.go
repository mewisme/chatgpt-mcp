package caveman

import "testing"

func TestRequestedState(t *testing.T) {
	for prompt, want := range map[string]bool{"/caveman": true, "@caveman": true, "$caveman off": false, "caveman mode": true, "caveman mode off": false, "stop caveman": false, "normal mode": false} {
		got, ok := RequestedState(prompt)
		if !ok || got != want {
			t.Fatalf("%q => %t %t, want %t", prompt, got, ok, want)
		}
	}
	if _, ok := RequestedState("normal user prompt"); ok {
		t.Fatal("normal prompt should not change caveman state")
	}
}

func TestManagerTurn(t *testing.T) {
	manager := NewManager()
	first, err := manager.Turn("workspace", "/caveman", "turn")
	if err != nil || !first.Active || first.ActiveInstructions == "" {
		t.Fatalf("activate = %#v %v", first, err)
	}
	second, err := manager.Turn("workspace", "continue", "turn")
	if err != nil || !second.Active || second.ActiveInstructions != "" || second.RefreshHint == "" {
		t.Fatalf("second = %#v %v", second, err)
	}
	off, err := manager.Turn("workspace", "stop caveman", "turn")
	if err != nil || off.Active {
		t.Fatalf("off = %#v %v", off, err)
	}
}
