package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeEntriesFindsOverlapFragmentsAndOversizedNotes(t *testing.T) {
	analysis := AnalyzeEntries([]Entry{
		{Scope: "tui", Key: "theme", Note: "Use Charm default styles and preserve automatic dark mode adaptation"},
		{Scope: "tui", Key: "colors", Note: "Use Charm default styles and preserve automatic dark mode colors"},
		{Scope: "tui", Key: "layout", Note: "Center layout"},
		{Scope: "tui", Key: "dialogs", Note: "Center dialogs"},
		{Scope: "release", Key: "notes", Note: strings.Repeat("long ", 100)},
	})
	reasons := map[string]bool{}
	for _, group := range analysis.Groups {
		reasons[group.Reason] = true
	}
	if !reasons["high semantic overlap candidate"] || !reasons["fragmented small entries"] || !reasons["oversized note"] || analysis.CandidateSavingsBytes <= 0 {
		t.Fatalf("analysis = %#v", analysis)
	}
}

func TestStoreAnalyzeDetectsLegacyFormatWithoutMutating(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	path := store.WorkspacePath("ws_test")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	legacy := "## tooling\n- use pnpm\n"
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	analysis, err := store.Analyze("ws_test", "")
	if err != nil {
		t.Fatal(err)
	}
	if !analysis.LegacyFormat || !analysis.OptimizationRecommended {
		t.Fatalf("analysis = %#v", analysis)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != legacy {
		t.Fatalf("analysis mutated memory: %q", raw)
	}
}

func TestCompactionRecommendedByScopeSizeMemorySizeOrOverlap(t *testing.T) {
	many := make([]Entry, 0, ScopeEntriesSoftLimit+1)
	for index := 0; index <= ScopeEntriesSoftLimit; index++ {
		many = append(many, Entry{Scope: "tui", Key: string(rune('a' + index)), Note: "unique note"})
	}
	if !CompactionRecommended(many, 0) {
		t.Fatal("scope soft limit did not recommend compaction")
	}
	if !CompactionRecommended([]Entry{{Scope: "general", Key: "one", Note: "short"}}, MemorySoftLimitBytes+1) {
		t.Fatal("memory byte soft limit did not recommend compaction")
	}
	overlap := []Entry{{Scope: "tui", Key: "theme", Note: "Use Charm default styles and preserve automatic dark mode adaptation"}, {Scope: "tui", Key: "colors", Note: "Use Charm default styles and preserve automatic dark mode colors"}}
	if !CompactionRecommended(overlap, 0) {
		t.Fatal("overlap did not recommend compaction")
	}
	if CompactionRecommended([]Entry{{Scope: "general", Key: "one", Note: "short unique note"}}, 20) {
		t.Fatal("small memory unexpectedly recommended compaction")
	}
}
