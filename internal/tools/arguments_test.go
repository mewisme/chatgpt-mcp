package tools

import (
	"encoding/json"
	"testing"
)

func TestIntegerArgumentsAcceptJSONNumber(t *testing.T) {
	args := map[string]any{"head_limit": json.Number("200"), "offset": json.Number("9007199254740993")}
	headLimit, err := optionalInt(args, "head_limit", 100, 1, 1000)
	if err != nil || headLimit != 200 {
		t.Fatalf("head_limit=%d err=%v", headLimit, err)
	}
	offset, err := optionalInt64(args, "offset", 0, 0, 1<<62)
	if err != nil || offset != 9007199254740993 {
		t.Fatalf("offset=%d err=%v", offset, err)
	}
}

func TestIntegerArgumentsRejectFractionalJSONNumber(t *testing.T) {
	if _, err := optionalInt(map[string]any{"head_limit": json.Number("200.5")}, "head_limit", 100, 1, 1000); err == nil {
		t.Fatal("fractional JSON number was accepted as an integer")
	}
}
