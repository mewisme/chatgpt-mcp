package pluginbuild

import "testing"

func TestShouldCompressLinuxAndWindowsOnly(t *testing.T) {
	for goos, want := range map[string]bool{"linux": true, "windows": true, "darwin": false, "js": false, "": false} {
		if got := ShouldCompress(goos); got != want {
			t.Fatalf("ShouldCompress(%q) = %v, want %v", goos, got, want)
		}
	}
}

func TestUPXDisabledOnlyOnExplicitOff(t *testing.T) {
	if !upxDisabled("off") || !upxDisabled("OFF") {
		t.Fatal("off should disable UPX")
	}
	if upxDisabled("") || upxDisabled("on") {
		t.Fatal("missing or other values must not disable UPX")
	}
}
