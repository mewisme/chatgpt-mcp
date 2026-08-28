package rules

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ruleRoots = []struct {
	Relative string
	Source   string
}{
	{filepath.Join(".claude", "rules"), ".claude"},
	{filepath.Join(".claudes", "rules"), ".claudes"},
	{filepath.Join(".agents", "rules"), ".agents"},
	{filepath.Join(".cursor", "rules"), ".cursor"},
	{filepath.Join(".codex", "rules"), ".codex"},
}

func Discover(workspaceRoot string) ([]Rule, error) {
	result := make([]Rule, 0)
	for _, root := range ruleRoots {
		walkRules(filepath.Join(workspaceRoot, root.Relative), root.Source, 0, &result)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func LoadForFile(workspaceRoot, file string) ([]Rule, error) {
	all, err := Discover(workspaceRoot)
	if err != nil {
		return nil, err
	}
	matched := make([]Rule, 0)
	for _, rule := range all {
		if Match(rule, workspaceRoot, file) {
			matched = append(matched, rule)
		}
	}
	return matched, nil
}

func walkRules(dir, source string, depth int, result *[]Rule) {
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
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			walkRules(path, source, depth+1, result)
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension != ".md" && extension != ".mdc" {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		patterns, alwaysApply, body := parseRule(string(data))
		if strings.TrimSpace(body) == "" {
			continue
		}
		if len(body) > 4000 {
			body = body[:4000]
		}
		*result = append(*result, Rule{
			Path: path, Source: source, Patterns: patterns, Content: body, AlwaysApply: alwaysApply,
		})
	}
}

func parseRule(raw string) ([]string, bool, string) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, false, strings.TrimSpace(normalized)
	}
	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, false, strings.TrimSpace(normalized)
	}
	header := rest[:end]
	body := strings.TrimSpace(rest[end+len("\n---"):])
	patterns := make([]string, 0)
	alwaysApply := false
	activeList := ""
	for _, line := range strings.Split(header, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "-") && activeList != "" {
			value := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "-")), `"'`)
			if value != "" {
				patterns = append(patterns, value)
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			activeList = ""
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "paths", "globs":
			activeList = key
			if value != "" {
				for _, item := range strings.Split(strings.Trim(value, "[]"), ",") {
					item = strings.Trim(strings.TrimSpace(item), `"'`)
					if item != "" {
						patterns = append(patterns, item)
					}
				}
			}
		case "alwaysApply", "always_apply":
			activeList = ""
			alwaysApply = strings.EqualFold(strings.Trim(value, `"'`), "true")
		default:
			activeList = ""
		}
	}
	return unique(patterns), alwaysApply, body
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
