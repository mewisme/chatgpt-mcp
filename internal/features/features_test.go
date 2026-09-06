package features

import "testing"

func TestDefaultActivatesBuiltInModes(t *testing.T) {
	value := Default()
	if !value.Ponytail.Active || value.Ponytail.Mode != "full" || !value.Caveman.Active || value.Caveman.Mode != "full" {
		t.Fatalf("default features = %#v", value)
	}
}
