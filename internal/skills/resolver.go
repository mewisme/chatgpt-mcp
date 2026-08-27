package skills

import (
	"os"
	"path/filepath"
)

type Resolver struct{}

func (Resolver) Discover(workdir string) []Skill {
	var result []Skill
	for _, root := range []string{filepath.Join(workdir, ".agents", "skills"), filepath.Join(workdir, ".claude", "skills")} {
		entries, _ := os.ReadDir(root)
		for _, entry := range entries { if entry.IsDir() { result = append(result, Skill{Name: entry.Name(), Path: filepath.Join(root, entry.Name())}) } }
	}
	return result
}
