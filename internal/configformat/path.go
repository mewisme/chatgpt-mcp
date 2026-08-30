package configformat

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Source struct {
	Path   string
	Format Format
	Ext    string
	Exists bool
}

func RootPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "chatgpt-mcp"
	}
	return filepath.Join(home, ".config", "chatgpt-mcp")
}

func Discover(root string) (Source, error) {
	if strings.TrimSpace(root) == "" {
		root = RootPath()
	}
	candidates := []string{"config.json", "config.yaml", "config.yml", "config.toml"}
	found := make([]string, 0, len(candidates))
	for _, name := range candidates {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			found = append(found, path)
		} else if !os.IsNotExist(err) {
			return Source{}, err
		}
	}
	if len(found) > 1 {
		sort.Strings(found)
		return Source{}, fmt.Errorf("multiple main config files found: %s", strings.Join(found, ", "))
	}
	if len(found) == 0 {
		return Source{Path: filepath.Join(root, "config.json"), Format: JSON, Ext: ".json", Exists: false}, nil
	}
	format, err := Detect(found[0])
	if err != nil {
		return Source{}, err
	}
	return Source{Path: found[0], Format: format, Ext: filepath.Ext(found[0]), Exists: true}, nil
}

func PathFor(root, name string, format Format) string {
	return filepath.Join(root, name+Extension(format))
}

func StructuredPath(root, name string) string {
	return filepath.Join(root, name+ExtensionForRoot(root))
}

func ExtensionForRoot(root string) string {
	source, err := Discover(root)
	if err != nil || source.Ext == "" {
		return ".json"
	}
	return source.Ext
}

func StructuredPathFrom(path, name string) string {
	root := filepath.Dir(path)
	ext := filepath.Ext(path)
	if _, err := Detect(path); err != nil {
		ext = ".json"
	}
	return filepath.Join(root, name+ext)
}
