package patch

import (
	"strings"
	"testing"
)

func TestApplyCodexHunk(t *testing.T) {
	original := "alpha\nbeta\ngamma\n"
	next, err := ApplyUnifiedPatchToText(original, "@@\n beta\n-gamma\n+delta\n")
	if err != nil {
		t.Fatal(err)
	}
	if next != "alpha\nbeta\ndelta\n" {
		t.Fatalf("next = %q", next)
	}
}

func TestApplyStandardHunk(t *testing.T) {
	original := "alpha\nbeta\ngamma\n"
	next, err := ApplyUnifiedPatchToText(original, "@@ -2,2 +2,2 @@\n beta\n-gamma\n+delta\n")
	if err != nil {
		t.Fatal(err)
	}
	if next != "alpha\nbeta\ndelta\n" {
		t.Fatalf("next = %q", next)
	}
}

func TestParseCodexMultiFilePatch(t *testing.T) {
	ops, err := ParseMultiFilePatch("*** Begin Patch\n*** Add File: a.txt\n+hello\n*** Update File: b.txt\n@@\n-old\n+new\n*** Delete File: c.txt\n*** End Patch", "/tmp/base")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 3 || ops[0].Operation != "create" || ops[1].Operation != "update" || ops[2].Operation != "delete" {
		t.Fatalf("ops = %#v", ops)
	}
	if !strings.HasSuffix(ops[0].Path, "a.txt") || ops[0].Content != "hello" {
		t.Fatalf("create op = %#v", ops[0])
	}
}
