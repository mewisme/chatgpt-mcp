//go:build linux

package service

import (
	"strings"
)

func PersistenceWarning(spec Spec) string {
	if spec.Scope != ScopeUser || spec.Account.Username == "" {
		return ""
	}
	output, err := runCommand("loginctl", "show-user", spec.Account.Username, "--property=Linger", "--value")
	if err != nil || !strings.EqualFold(strings.TrimSpace(output), "no") {
		return ""
	}
	return "User service may stop when the last login session ends"
}
