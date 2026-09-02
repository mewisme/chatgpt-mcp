package instructioncontext

import "testing"

func canonicalTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := canonicalEnvironmentPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
