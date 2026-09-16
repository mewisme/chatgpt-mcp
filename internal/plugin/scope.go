package plugin

import (
	"errors"
	"fmt"
	"strings"
)

type PluginScope string

const (
	ScopeGlobal    PluginScope = "global"
	ScopeWorkspace PluginScope = "workspace"
)

var ErrScopeNotAllowed = errors.New("plugin does not allow this install scope")

func ParsePluginScope(value string) (PluginScope, error) {
	switch PluginScope(strings.TrimSpace(value)) {
	case ScopeGlobal:
		return ScopeGlobal, nil
	case ScopeWorkspace:
		return ScopeWorkspace, nil
	default:
		return "", fmt.Errorf("unknown plugin scope: %q", value)
	}
}

func defaultPluginScopes() []PluginScope {
	return []PluginScope{ScopeGlobal}
}

func clonePluginScopes(scopes []PluginScope) []PluginScope {
	if len(scopes) == 0 {
		return defaultPluginScopes()
	}
	return append([]PluginScope(nil), scopes...)
}

func validatePluginScopes(scopes []PluginScope) error {
	if len(scopes) == 0 {
		return fmt.Errorf("plugin must declare at least one scope")
	}
	seen := map[PluginScope]struct{}{}
	for _, scope := range scopes {
		if _, err := ParsePluginScope(string(scope)); err != nil {
			return err
		}
		if _, ok := seen[scope]; ok {
			return fmt.Errorf("duplicate plugin scope: %q", scope)
		}
		seen[scope] = struct{}{}
	}
	return nil
}

func (manifest Manifest) AllowedScopes() []PluginScope {
	return clonePluginScopes(manifest.Scopes)
}

func (manifest Manifest) AllowsScope(scope PluginScope) bool {
	for _, allowed := range manifest.AllowedScopes() {
		if allowed == scope {
			return true
		}
	}
	return false
}

func (builtin Builtin) AllowedScopes() []PluginScope {
	return clonePluginScopes(builtin.Scopes)
}

func (entry RegistryEntry) AllowedScopes() []PluginScope {
	return clonePluginScopes(entry.Scopes)
}
