package plugin

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type PayloadRule struct {
	Name string
	Path string
}

type PayloadSkill struct {
	Name        string
	Path        string
	Description string
	Files       []string
}

type PayloadResources struct {
	Rules  []PayloadRule
	Skills []PayloadSkill
}

func DiscoverPayloadResources(payloadDir string) (PayloadResources, error) {
	payloadDir = filepath.Clean(strings.TrimSpace(payloadDir))
	if payloadDir == "" {
		return PayloadResources{}, errors.New("plugin payload directory is required")
	}
	info, err := os.Lstat(payloadDir)
	if err != nil {
		return PayloadResources{}, fmt.Errorf("inspect plugin payload: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return PayloadResources{}, errors.New("plugin payload must not be a symlink")
	}
	if !info.IsDir() {
		return PayloadResources{}, errors.New("plugin payload must be a directory")
	}
	rules, err := discoverPayloadRules(payloadDir)
	if err != nil {
		return PayloadResources{}, err
	}
	skills, err := discoverPayloadSkills(payloadDir)
	if err != nil {
		return PayloadResources{}, err
	}
	return PayloadResources{Rules: rules, Skills: skills}, nil
}

func discoverPayloadRules(payloadDir string) ([]PayloadRule, error) {
	dir := filepath.Join(payloadDir, "rules")
	entries, err := readPayloadDir(dir, "plugin rules")
	if err != nil || entries == nil {
		return nil, err
	}
	var rules []PayloadRule
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dir, name)
		if err := rejectSpecialEntry(entry, "plugin rule"); err != nil {
			return nil, err
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		stem := strings.TrimSuffix(name, ".md")
		if !validCanonicalName(stem) {
			return nil, fmt.Errorf("plugin rule name %q is invalid", stem)
		}
		if !pathWithin(payloadDir, path) {
			return nil, fmt.Errorf("plugin rule %s escapes payload", name)
		}
		rules = append(rules, PayloadRule{Name: stem, Path: "rules/" + name})
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	return rules, nil
}

func discoverPayloadSkills(payloadDir string) ([]PayloadSkill, error) {
	dir := filepath.Join(payloadDir, "skills")
	entries, err := readPayloadDir(dir, "plugin skills")
	if err != nil || entries == nil {
		return nil, err
	}
	var skills []PayloadSkill
	for _, entry := range entries {
		name := entry.Name()
		skillDir := filepath.Join(dir, name)
		if err := rejectSpecialEntry(entry, "plugin skill"); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		if !validCanonicalName(name) {
			return nil, fmt.Errorf("plugin skill name %q is invalid", name)
		}
		skillPath := filepath.Join(skillDir, "SKILL.md")
		info, err := os.Lstat(skillPath)
		if err != nil {
			return nil, fmt.Errorf("plugin skill %s is missing SKILL.md", name)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("plugin skill %s SKILL.md is a symlink", name)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("plugin skill %s SKILL.md is not a regular file", name)
		}
		if !pathWithin(payloadDir, skillPath) {
			return nil, fmt.Errorf("plugin skill %s escapes payload", name)
		}
		content, err := os.ReadFile(skillPath)
		if err != nil {
			return nil, fmt.Errorf("read plugin skill %s: %w", name, err)
		}
		frontName, description := parseSkillFrontmatter(string(content))
		if frontName != name {
			return nil, fmt.Errorf("plugin skill %s frontmatter name %q must match directory", name, frontName)
		}
		if strings.TrimSpace(description) == "" {
			return nil, fmt.Errorf("plugin skill %s is missing a description", name)
		}
		files, err := collectSkillFiles(payloadDir, skillDir)
		if err != nil {
			return nil, err
		}
		skills = append(skills, PayloadSkill{
			Name:        name,
			Path:        "skills/" + name + "/SKILL.md",
			Description: description,
			Files:       files,
		})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

func collectSkillFiles(payloadDir, skillDir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(skillDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !pathWithin(payloadDir, path) || !pathWithin(skillDir, path) {
			return fmt.Errorf("plugin skill path %s escapes payload", path)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			rel, _ := filepath.Rel(skillDir, path)
			return fmt.Errorf("plugin skill file %s is a symlink", filepath.ToSlash(rel))
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			rel, _ := filepath.Rel(skillDir, path)
			return fmt.Errorf("plugin skill file %s is not a regular file", filepath.ToSlash(rel))
		}
		rel, err := filepath.Rel(skillDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "SKILL.md" {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func readPayloadDir(path, label string) ([]os.DirEntry, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s directory must not be a symlink", label)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s must be a directory", label)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	return entries, nil
}

func rejectSpecialEntry(entry os.DirEntry, label string) error {
	if entry.Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s %s is a symlink", label, entry.Name())
	}
	info, err := entry.Info()
	if err != nil {
		return fmt.Errorf("inspect %s %s: %w", label, entry.Name(), err)
	}
	mode := info.Mode()
	if mode&os.ModeSymlink != 0 {
		return fmt.Errorf("%s %s is a symlink", label, entry.Name())
	}
	if !mode.IsDir() && !mode.IsRegular() {
		return fmt.Errorf("%s %s is not a regular file", label, entry.Name())
	}
	return nil
}

func parseSkillFrontmatter(content string) (string, string) {
	if !strings.HasPrefix(content, "---") {
		return "", ""
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}
	name := ""
	description := ""
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			name = strings.Trim(strings.TrimSpace(value), `"'`)
		case "description":
			description = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return name, description
}
