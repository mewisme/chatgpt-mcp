package rules

import (
	"path/filepath"
	"regexp"
	"strings"
)

type Rule struct {
	Path        string   `json:"path"`
	Source      string   `json:"source"`
	Patterns    []string `json:"patterns,omitempty"`
	Content     string   `json:"content"`
	AlwaysApply bool     `json:"always_apply,omitempty"`
}

func Match(rule Rule, workspaceRoot, file string) bool {
	if rule.AlwaysApply {
		return true
	}
	if len(rule.Patterns) == 0 {
		return false
	}
	absolute := filepath.ToSlash(filepath.Clean(file))
	relative, err := filepath.Rel(workspaceRoot, file)
	if err != nil {
		relative = file
	}
	relative = filepath.ToSlash(relative)
	base := filepath.Base(absolute)
	for _, pattern := range rule.Patterns {
		regex, err := globRegex(pattern)
		if err != nil {
			continue
		}
		if regex.MatchString(relative) || regex.MatchString(absolute) || regex.MatchString(base) {
			return true
		}
	}
	return false
}

func globRegex(pattern string) (*regexp.Regexp, error) {
	pattern = filepath.ToSlash(strings.Trim(strings.TrimSpace(pattern), `"'`))
	var builder strings.Builder
	builder.WriteString("(?i)^")
	for i := 0; i < len(pattern); {
		if i+1 < len(pattern) && pattern[i:i+2] == "**" {
			builder.WriteString(".*")
			i += 2
			continue
		}
		switch pattern[i] {
		case '*':
			builder.WriteString(`[^/]*`)
		case '?':
			builder.WriteString(".")
		case '{':
			end := strings.IndexByte(pattern[i:], '}')
			if end > 0 {
				inner := pattern[i+1 : i+end]
				parts := strings.Split(inner, ",")
				builder.WriteString("(")
				for index, part := range parts {
					if index > 0 {
						builder.WriteString("|")
					}
					builder.WriteString(regexp.QuoteMeta(strings.TrimSpace(part)))
				}
				builder.WriteString(")")
				i += end + 1
				continue
			}
			builder.WriteString(regexp.QuoteMeta(string(pattern[i])))
		default:
			builder.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
		i++
	}
	builder.WriteString("$")
	return regexp.Compile(builder.String())
}
