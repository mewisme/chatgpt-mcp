package commandpattern

import "testing"

func TestPatternPrefixAndGlobMatching(t *testing.T) {
	tests := []struct {
		pattern string
		argv    []string
		want    bool
	}{
		{"git push", []string{"git", "push", "origin", "main"}, true},
		{"git push", []string{"echo", "git", "push"}, false},
		{"git *", []string{"git", "status"}, true},
		{"git *", []string{"git", "push", "origin", "main"}, true},
		{"go test ./...", []string{"go", "test", "./...", "-count=1"}, true},
		{"rm -rf *", []string{"rm", "-rf", "build/cache", "other"}, true},
		{"docker ** rm *", []string{"docker", "--context", "local", "container", "rm", "app"}, true},
		{"docker ** rm *", []string{"docker", "ps"}, false},
		{"tool file?.[ch]", []string{"tool", "file1.c"}, true},
		{"tool file?.[ch]", []string{"tool", "file12.c"}, false},
	}
	for _, test := range tests {
		pattern, err := Parse(test.pattern)
		if err != nil {
			t.Fatalf("Parse(%q): %v", test.pattern, err)
		}
		if got := pattern.Match(test.argv); got != test.want {
			t.Fatalf("%q match %#v = %t want %t", test.pattern, test.argv, got, test.want)
		}
	}
}

func TestPatternQuotedTokensAndValidation(t *testing.T) {
	pattern, err := Parse(`git commit -m "release *"`)
	if err != nil {
		t.Fatal(err)
	}
	if !pattern.Match([]string{"git", "commit", "-m", "release 0.2.5"}) {
		t.Fatal("quoted glob token did not match one argv value")
	}
	for _, invalid := range []string{"", `git "status`, "git [abc"} {
		if _, err := Parse(invalid); err == nil {
			t.Fatalf("invalid pattern accepted: %q", invalid)
		}
	}
}

func TestCommandWordsRejectsCompoundShellCommands(t *testing.T) {
	values, err := CommandWords(`git push origin main`)
	if err != nil || len(values) != 4 || values[0] != "git" || values[1] != "push" {
		t.Fatalf("words=%#v err=%v", values, err)
	}
	for _, value := range []string{`git push && rm -rf build`, `git push | tee log`, `git push > out`} {
		if _, err := CommandWords(value); err == nil {
			t.Fatalf("compound command accepted: %q", value)
		}
	}
}
