package memory

import "testing"

func TestMemoryIndexRebuildUpsertDelete(t *testing.T) {
	index := NewMemoryIndex()
	if err := index.Rebuild("ws_test", []Entry{{Scope: "tui", Key: "theme", Note: "Use Charm defaults"}, {Scope: "coding-style", Key: "imports", Note: "Keep imports contiguous"}}); err != nil {
		t.Fatal(err)
	}
	matches, err := index.Search("ws_test", Query{})
	if err != nil || len(matches) != 2 {
		t.Fatalf("matches=%#v err=%v", matches, err)
	}
	if err := index.Upsert("ws_test", Entry{Scope: "tui", Key: "layout", Note: "Center layout"}); err != nil {
		t.Fatal(err)
	}
	matches, err = index.Search("ws_test", Query{Scope: "tui"})
	if err != nil || len(matches) != 2 {
		t.Fatalf("matches=%#v err=%v", matches, err)
	}
	if err := index.Delete("ws_test", "tui", "theme"); err != nil {
		t.Fatal(err)
	}
	matches, _ = index.Search("ws_test", Query{Scope: "tui"})
	if len(matches) != 1 || matches[0].Entry.Key != "layout" {
		t.Fatalf("matches=%#v", matches)
	}
	if err := index.Delete("ws_test", "tui", ""); err != nil {
		t.Fatal(err)
	}
	matches, _ = index.Search("ws_test", Query{})
	if len(matches) != 1 || matches[0].Entry.Scope != "coding-style" {
		t.Fatalf("matches=%#v", matches)
	}
}

func TestMemoryIndexIsWorkspaceIsolated(t *testing.T) {
	index := NewMemoryIndex()
	_ = index.Rebuild("ws_a", []Entry{{Scope: "general", Key: "one", Note: "alpha"}})
	_ = index.Rebuild("ws_b", []Entry{{Scope: "general", Key: "two", Note: "beta"}})
	a, _ := index.Search("ws_a", Query{})
	b, _ := index.Search("ws_b", Query{})
	if len(a) != 1 || a[0].Entry.Key != "one" || len(b) != 1 || b[0].Entry.Key != "two" {
		t.Fatalf("a=%#v b=%#v", a, b)
	}
}

func TestMemoryIndexSearchRanksKeyAndScopeMatches(t *testing.T) {
	index := NewMemoryIndex()
	_ = index.Rebuild("ws_test", []Entry{
		{Scope: "tui", Key: "theme", Note: "Use component defaults"},
		{Scope: "ui", Key: "colors", Note: "Theme settings"},
		{Scope: "release", Key: "ci", Note: "GitHub actions"},
	})
	matches, err := index.Search("ws_test", Query{Text: "tui theme", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].Entry.Scope != "tui" || matches[0].Entry.Key != "theme" || matches[0].Score <= matches[1].Score {
		t.Fatalf("matches=%#v", matches)
	}
}
