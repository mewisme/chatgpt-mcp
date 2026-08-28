package ponytail

import "testing"

func TestRequestedMode(t *testing.T) {
	cases := map[string]Mode{
		"/ponytail lite":             Lite,
		"@ponytail full":             Full,
		"$ponytail ultra":            Ultra,
		"/ponytail-review":           Review,
		"/ponytail :ponytail-review": Review,
		"/ponytail off":              Off,
		"stop ponytail":              Off,
		"normal mode":                Off,
	}
	for prompt, expected := range cases {
		value, ok := RequestedMode(prompt)
		if !ok || value != expected {
			t.Fatalf("%q => %q %v, want %q", prompt, value, ok, expected)
		}
	}
	if _, ok := RequestedMode("normal user prompt"); ok {
		t.Fatal("normal prompt should not request a mode")
	}
}

func TestModeFromInstructions(t *testing.T) {
	if value := ModeFromInstructions("PONYTAIL MODE ACTIVE — level: review"); value != Review {
		t.Fatalf("mode = %q", value)
	}
	if value := ModeFromInstructions("no marker"); value != Full {
		t.Fatalf("default mode = %q", value)
	}
}
