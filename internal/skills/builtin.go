package skills

import (
	"embed"
	"io/fs"
	"path"
	"sort"
	"strings"
)

const BuiltinSource = "builtin"
const builtinPathPrefix = "builtin://"

var reservedBuiltinNames = map[string]bool{
	"create-skill": true,
	"create-rule":  true,
}

//go:embed builtins/*/SKILL.md
var builtinFS embed.FS

func Builtin(skill Skill) bool {
	return skill.Source == BuiltinSource || strings.HasPrefix(skill.Path, builtinPathPrefix)
}

func mergeBuiltins(values []Skill) []Skill {
	builtins := builtinSkills()
	result := append([]Skill(nil), builtins...)
	for _, skill := range values {
		if reservedBuiltinNames[skill.Name] {
			continue
		}
		result = append(result, skill)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func builtinSkills() []Skill {
	entries, err := fs.ReadDir(builtinFS, "builtins")
	if err != nil {
		return nil
	}
	result := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !reservedBuiltinNames[entry.Name()] {
			continue
		}
		content, err := builtinContent(entry.Name())
		if err != nil {
			continue
		}
		name, description := parseFrontmatter(content)
		if name == "" {
			name = entry.Name()
		}
		if description == "" {
			description = name
		}
		result = append(result, Skill{Name: name, Description: description, Path: builtinPathPrefix + name + "/SKILL.md", Source: BuiltinSource})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func builtinContent(name string) (string, error) {
	data, err := builtinFS.ReadFile(path.Join("builtins", name, "SKILL.md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
