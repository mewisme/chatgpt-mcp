package memory

import "testing"

func TestLocalEmbedderProducesNormalizedDeterministicVectors(t *testing.T) {
	embedder := NewLocalEmbedder()
	vectors, err := embedder.Embed(nil, []string{"Charm default theme", "Charm default theme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 || len(vectors[0]) != DefaultEmbeddingDimensions {
		t.Fatalf("vectors = %#v", vectors)
	}
	if cosine(vectors[0], vectors[1]) < 0.999 {
		t.Fatalf("identical embedding cosine = %f", cosine(vectors[0], vectors[1]))
	}
}

func TestHybridIndexRanksEmbeddingAndLexicalSignals(t *testing.T) {
	index := NewHybridIndex(NewLocalEmbedder(), DefaultHybridWeights())
	entries := []Entry{
		{Scope: "tui", Key: "theme", Note: "Use Charm component native default palette"},
		{Scope: "release", Key: "ci", Note: "Publish artifacts with GitHub Actions"},
	}
	if err := index.Rebuild("ws_test", entries); err != nil {
		t.Fatal(err)
	}
	matches, err := index.Search("ws_test", Query{Text: "Charm component theme", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 || matches[0].Entry.Key != "theme" || matches[0].Score <= 0 {
		t.Fatalf("matches = %#v", matches)
	}
}

func TestHybridIndexImplementsIndex(t *testing.T) {
	var _ Index = NewHybridIndex(nil, HybridWeights{})
}
