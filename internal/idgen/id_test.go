package idgen

import (
	"regexp"
	"testing"
)

func TestNewUsesPrefixedHex(t *testing.T) {
	value, err := New("run", 8)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^run_[0-9a-f]{16}$`).MatchString(value) {
		t.Fatalf("id=%q", value)
	}
}
