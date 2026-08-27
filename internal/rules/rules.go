package rules

import "strings"

type Rule struct {
	Path     string
	Source   string
	Patterns []string
	Content  string
}

func Match(rule Rule, file string) bool {
	for _, pattern := range rule.Patterns {
		if strings.Contains(file, strings.Trim(pattern, "*")) {
			return true
		}
	}
	return false
}
