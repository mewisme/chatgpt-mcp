package features

import "testing"

func TestDefaultEnablesBuiltInFeatures(t *testing.T) {
	value := Default()
	if !value.Ponytail.Enabled || !value.Caveman.Enabled {
		t.Fatalf("default features = %#v", value)
	}
}
