package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRenderCanonicalMemoryRoundTrip(t *testing.T) {
	input := "## tui\n\n### theme\n- Use Charm defaults.\n\n### layout\n- Center the main layout.\n\n## coding-style\n\n### imports\n- Keep imports contiguous.\n"
	document := Parse(input)
	if len(document.Entries) != 3 {
		t.Fatalf("entries = %#v", document.Entries)
	}
	rendered := Render(document)
	if reparsed := Parse(rendered); Render(reparsed) != rendered {
		t.Fatalf("round trip changed\nfirst:\n%s\nsecond:\n%s", rendered, Render(reparsed))
	}
}

func TestParseDeduplicatesCaseInsensitiveScopeKey(t *testing.T) {
	document := Parse("## TUI\n### Theme\n- Use Charm defaults.\n## tui\n### theme\n- Preserve light and dark adaptation.\n")
	if len(document.Entries) != 1 {
		t.Fatalf("entries = %#v", document.Entries)
	}
	entry := document.Entries[0]
	if entry.Scope != "TUI" || entry.Key != "Theme" || entry.Note != "Use Charm defaults. Preserve light and dark adaptation." {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestParseMigratesScopedParagraphFormatToGeneralKey(t *testing.T) {
	document := Parse("## tooling\n- use pnpm\n\n## tui\n- use Charm defaults\n")
	want := "## tooling\n\n### general\n- use pnpm\n\n## tui\n\n### general\n- use Charm defaults\n"
	if rendered := Render(document); rendered != want {
		t.Fatalf("rendered = %q, want %q", rendered, want)
	}
}

func TestParseMigratesLegacyDateNotesToGeneralEntry(t *testing.T) {
	document := Parse("# Auto memory (cross-session notes)\n\n- 2026-09-04: use compact imports\n- 2026-09-05: prefer pnpm\n")
	want := "## general\n\n### general\n- use compact imports prefer pnpm\n"
	if rendered := Render(document); rendered != want {
		t.Fatalf("rendered = %q, want %q", rendered, want)
	}
}

func TestParseNormalizesWhitespaceAndWrappedNotes(t *testing.T) {
	document := Parse("##  coding-style  \n### imports\n- Keep imports\n  contiguous   and compact.\n")
	if len(document.Entries) != 1 || document.Entries[0] != (Entry{Scope: "coding-style", Key: "imports", Note: "Keep imports contiguous and compact."}) {
		t.Fatalf("entries = %#v", document.Entries)
	}
}

func TestRenderUsesDeterministicScopeAndKeyOrdering(t *testing.T) {
	document := Document{Entries: []Entry{{Scope: "tui", Key: "theme", Note: "theme"}, {Scope: "coding-style", Key: "typescript", Note: "ts"}, {Scope: "coding-style", Key: "imports", Note: "imports"}}}
	rendered := Render(document)
	if strings.Index(rendered, "## coding-style") > strings.Index(rendered, "## tui") || strings.Index(rendered, "### imports") > strings.Index(rendered, "### typescript") {
		t.Fatalf("non-deterministic ordering:\n%s", rendered)
	}
}

func TestStoreLoadDocumentAndSaveDocument(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	path, err := store.SaveDocument("ws_test", Document{Entries: []Entry{{Scope: "tooling", Key: "package-manager", Note: "use pnpm"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, filepath.Join("ws_test", "MEMORY.md")) {
		t.Fatalf("path = %q", path)
	}
	document, err := store.LoadDocument("ws_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Entries) != 1 || document.Entries[0].Key != "package-manager" {
		t.Fatalf("document = %#v", document)
	}
}

func TestStoreMigrationPersistsOnlyOnSave(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	path := store.WorkspacePath("ws_test")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	legacy := "## tooling\n- use pnpm\n"
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	document, err := store.LoadDocument("ws_test")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != legacy {
		t.Fatalf("load persisted migration: %q", raw)
	}
	if _, err := store.SaveDocument("ws_test", document); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "### general") {
		t.Fatalf("save did not persist migration: %q", raw)
	}
}

func TestUpsertInsertsAndReplacesCanonicalEntry(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	if _, err := store.Upsert("ws_test", "tui", "theme", "Use Nord colors."); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Upsert("ws_test", "TUI", "THEME", "Use Charm default component styles."); err != nil {
		t.Fatal(err)
	}
	document, err := store.LoadDocument("ws_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Entries) != 1 || document.Entries[0].Note != "Use Charm default component styles." {
		t.Fatalf("document = %#v", document)
	}
}

func TestUpsertAllowsSameKeyInDifferentScopes(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	if _, err := store.Upsert("ws_test", "tui", "theme", "Charm"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Upsert("ws_test", "web", "theme", "System"); err != nil {
		t.Fatal(err)
	}
	document, err := store.LoadDocument("ws_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Entries) != 2 {
		t.Fatalf("document = %#v", document)
	}
}

func TestUpsertRequiresScopeKeyAndNote(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	for _, args := range [][3]string{{"", "key", "note"}, {"scope", "", "note"}, {"scope", "key", ""}} {
		if _, err := store.Upsert("ws_test", args[0], args[1], args[2]); err == nil {
			t.Fatalf("expected error for %#v", args)
		}
	}
}
