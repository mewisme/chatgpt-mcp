package instructioncontext

import (
	"path/filepath"
	"sort"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/instructionpolicy"
	"go.mewis.me/chatgpt-mcp/internal/skills"
)

func nativeGlobalPath(path string) bool {
	root := strings.TrimSpace(configformat.RootPath())
	if root == "" || strings.TrimSpace(path) == "" {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

var skillSourcePriority = map[string]int{
	".agents":  0,
	".claude":  1,
	".claudes": 2,
	".cursor":  3,
	".codex":   4,
}

func LoadSkillSummaries(root string) ([]skills.Skill, error) {
	values, err := skills.Discover(root)
	return filterSkillSummaries(values, err)
}

func LoadSkillSummariesWithUser(root, home string, policy instructionpolicy.Config) ([]skills.Skill, error) {
	values, err := skills.DiscoverWithUser(root, home, policy)
	return filterSkillSummaries(values, err)
}

func filterSkillSummaries(values []skills.Skill, err error) ([]skills.Skill, error) {
	if err != nil {
		return nil, err
	}
	sort.SliceStable(values, func(i, j int) bool {
		left, right := skillPriority(values[i]), skillPriority(values[j])
		if left != right {
			return left < right
		}
		if values[i].Name != values[j].Name {
			return values[i].Name < values[j].Name
		}
		return values[i].Path < values[j].Path
	})
	seen := map[string]bool{}
	result := make([]skills.Skill, 0, len(values))
	for _, skill := range values {
		key := strings.Join([]string{strings.TrimSpace(skill.Name), strings.TrimSpace(skill.Description), strings.TrimSpace(skill.Source)}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, skill)
	}
	return result, nil
}

func skillPriority(skill skills.Skill) int {
	if skill.Source == skills.NativeSource {
		if nativeGlobalPath(skill.Path) {
			return len(skillSourcePriority) + 1
		}
		return -1
	}
	if priority, ok := skillSourcePriority[skill.Source]; ok {
		return priority
	}
	return len(skillSourcePriority)
}
