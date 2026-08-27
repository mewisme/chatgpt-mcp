package rules

import (
	"os"
	"path/filepath"
)

func Discover(workdir string) []Rule {
	var result []Rule
	root := filepath.Join(workdir, ".claude", "rules")
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if !entry.IsDir() {
			result = append(result, Rule{Path: filepath.Join(root, entry.Name())})
		}
	}
	return result
}
