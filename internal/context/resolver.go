package context

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type File struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

type Result struct {
	Root  string `json:"root"`
	Files []File `json:"files"`
	Count int    `json:"count"`
}

var contextCandidates = []string{
	"CLAUDE.md",
	"AGENTS.md",
	"README.md",
	filepath.Join(".claude", "settings.json"),
	filepath.Join(".codex", "config.toml"),
}

var providerRuleDirs = []string{
	filepath.Join(".claude", "rules"),
	filepath.Join(".claudes", "rules"),
	filepath.Join(".agents", "rules"),
	filepath.Join(".cursor", "rules"),
	filepath.Join(".codex", "rules"),
}

func Load(root string, maxDepth, maxBytesPerFile int) (Result, error) {
	if maxDepth < 0 {
		maxDepth = 0
	}
	if maxBytesPerFile <= 0 {
		maxBytesPerFile = 60_000
	}
	found := map[string]bool{}
	files := make([]File, 0)

	var addFile func(string)
	addFile = func(path string) {
		path = filepath.Clean(path)
		if found[path] {
			return
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		truncated := len(data) > maxBytesPerFile
		if truncated {
			data = data[:maxBytesPerFile]
		}
		found[path] = true
		files = append(files, File{Path: path, Content: string(data), Truncated: truncated})
	}

	var scanRules func(string, int)
	scanRules = func(dir string, depth int) {
		if depth > 3 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			full := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				scanRules(full, depth+1)
				continue
			}
			if ruleLike(entry.Name()) {
				addFile(full)
			}
		}
	}

	var walk func(string, int)
	walk = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}
		for _, name := range contextCandidates {
			addFile(filepath.Join(dir, name))
		}
		for _, relative := range providerRuleDirs {
			scanRules(filepath.Join(dir, relative), 0)
		}
		if depth == maxDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			if strings.HasPrefix(entry.Name(), ".") || skipDirectory(entry.Name()) {
				continue
			}
			walk(filepath.Join(dir, entry.Name()), depth+1)
		}
	}
	walk(root, 0)

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return Result{Root: filepath.Clean(root), Files: files, Count: len(files)}, nil
}

func ruleLike(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".mdc", ".txt", ".json", ".toml":
		return true
	default:
		return false
	}
}

func skipDirectory(name string) bool {
	switch name {
	case "node_modules", "dist", "build", "vendor", ".git":
		return true
	default:
		return false
	}
}

func ReadFileLimited(path string, maxBytes int) (string, bool, error) {
	if maxBytes <= 0 {
		return "", false, errors.New("max bytes must be positive")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", false, errors.New("path is not a regular non-symlink file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	truncated := len(data) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	return string(data), truncated, nil
}
