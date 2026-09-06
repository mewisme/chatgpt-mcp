package features

import "testing"

func TestDefaultActivatesBuiltInModes(t *testing.T) {
	value := Default()
	if !value.Ponytail.Active || !value.Caveman.Active {
		t.Fatalf("default features = %#v", value)
	}
}
