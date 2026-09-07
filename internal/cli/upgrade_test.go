package cli

import "testing"

func TestUpgradeIsCanonicalCommandAndUpdateIsAlias(t *testing.T) {
	root := newRootCommand()
	canonical, _, err := root.Find([]string{"upgrade", "check"})
	if err != nil {
		t.Fatal(err)
	}
	alias, _, err := root.Find([]string{"update", "check"})
	if err != nil {
		t.Fatal(err)
	}
	if canonical != alias {
		t.Fatal("update alias resolved to a different command")
	}
	if got := canonical.CommandPath(); got != root.Name()+" upgrade check" {
		t.Fatalf("canonical command path=%q", got)
	}
}
