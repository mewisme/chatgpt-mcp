package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestReplaceOwnedIsAtomicAndRemovesStaleTools(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister("native", Schema{Name: "native", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, map[string]any) (Result, error) {
		return TextResult("native"), nil
	})
	if err := registry.ReplaceOwned("upstream:a", map[string]Entry{
		"a__one": {Schema: Schema{Name: "a__one"}, Handler: func(context.Context, map[string]any) (Result, error) { return TextResult("one"), nil }},
		"a__two": {Schema: Schema{Name: "a__two"}, Handler: func(context.Context, map[string]any) (Result, error) { return TextResult("two"), nil }},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.ReplaceOwned("upstream:a", map[string]Entry{
		"a__two": {Schema: Schema{Name: "a__two"}, Handler: func(context.Context, map[string]any) (Result, error) { return TextResult("two"), nil }},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Call(context.Background(), "a__one", nil); err == nil {
		t.Fatal("stale owned tool still exists")
	}
	if _, err := registry.Call(context.Background(), "native", nil); err != nil {
		t.Fatal("native tool was removed")
	}
}
func TestRegistryOwnedReplacementSignalsChange(t *testing.T) {
	registry := NewRegistry()
	changes := registry.SubscribeChanges()
	defer registry.UnsubscribeChanges(changes)
	if err := registry.ReplaceOwned("upstream:test", map[string]Entry{
		"test__one": {Schema: Schema{Name: "test__one"}, Handler: func(context.Context, map[string]any) (Result, error) { return TextResult("one"), nil }},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changes:
	default:
		t.Fatal("owned replacement did not signal registry change")
	}
}
