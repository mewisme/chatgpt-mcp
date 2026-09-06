package capability

import "testing"

func TestCatalogHasUniqueIDsAndPaths(t *testing.T) {
	ids, paths := map[ID]bool{}, map[string]ID{}
	for _, spec := range All() {
		if spec.ID == "" || NormalizePath(spec.CanonicalPath) == "" {
			t.Fatalf("invalid capability: %#v", spec)
		}
		if ids[spec.ID] {
			t.Fatalf("duplicate capability id: %s", spec.ID)
		}
		ids[spec.ID] = true
		for _, path := range append([]string{spec.CanonicalPath}, spec.PublicPaths...) {
			path = NormalizePath(path)
			if previous, ok := paths[path]; ok && previous != spec.ID {
				t.Fatalf("path %q maps to both %s and %s", path, previous, spec.ID)
			}
			paths[path] = spec.ID
			if got, ok := ForPath(path); !ok || got != spec.ID {
				t.Fatalf("ForPath(%q)=%q,%t want %q", path, got, ok, spec.ID)
			}
		}
	}
}
