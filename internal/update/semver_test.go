package update

import (
	"errors"
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	for input, want := range map[string]string{
		"1.2.3":          "v1.2.3",
		"V1.2.3":         "v1.2.3",
		"v1.2.3":         "v1.2.3",
		"v1.2.3-rc.1":    "v1.2.3-rc.1",
		"v1.2.3+build.7": "v1.2.3",
	} {
		got, err := NormalizeVersion(input)
		if err != nil {
			t.Fatalf("NormalizeVersion(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
	for _, input := range []string{"", "v1", "v1.2", "v01.2.3", "v1.02.3", "v1.2.03", "v1.2.3-01", "v1.2.3-", "v1.2.3+"} {
		if _, err := NormalizeVersion(input); !errors.Is(err, ErrInvalidVersion) {
			t.Fatalf("NormalizeVersion(%q) error = %v", input, err)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.0", "v1.0.1", -1},
		{"v2.0.0", "v1.9.9", 1},
		{"v1.0.0-alpha", "v1.0.0", -1},
		{"v1.0.0-alpha", "v1.0.0-alpha.1", -1},
		{"v1.0.0-alpha.1", "v1.0.0-alpha.beta", -1},
		{"v1.0.0-beta.2", "v1.0.0-beta.11", -1},
		{"v1.0.0-rc.1", "v1.0.0", -1},
		{"1.2.3+one", "v1.2.3+two", 0},
	}
	for _, test := range tests {
		got, err := CompareVersions(test.left, test.right)
		if err != nil {
			t.Fatalf("CompareVersions(%q, %q): %v", test.left, test.right, err)
		}
		if got != test.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}
