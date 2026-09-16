package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	ErrProjectionConflict = errors.New("plugin resource destination is unmanaged or owned by another plugin")
	ErrProjectionDrift    = errors.New("plugin resource was modified locally")
)

type projectionKind string

const (
	projectionRule  projectionKind = "rule"
	projectionSkill projectionKind = "skill"
)

type projectedItem struct {
	Kind        projectionKind
	Name        string
	Source      string
	Destination string
	Digest      string
}

func SyncProjections(layout Layout, currentPayload, nextPayload string) error {
	if err := layout.Validate(); err != nil {
		return err
	}
	current, err := loadProjectedItems(layout, currentPayload)
	if err != nil {
		return err
	}
	next, err := loadProjectedItems(layout, nextPayload)
	if err != nil {
		return err
	}
	if err := preflightProjections(current, next); err != nil {
		return err
	}
	for _, dest := range sortedDests(next) {
		item := next[dest]
		actual, exists, err := destinationDigest(item)
		if err != nil {
			return err
		}
		if exists && actual == item.Digest {
			continue
		}
		if err := projectItem(item); err != nil {
			return err
		}
	}
	for _, dest := range sortedDests(current) {
		if _, keep := next[dest]; keep {
			continue
		}
		item := current[dest]
		actual, exists, err := destinationDigest(item)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if actual != item.Digest {
			return fmt.Errorf("%w: %s %s", ErrProjectionDrift, item.Kind, item.Name)
		}
		if err := removeProjectedItem(item); err != nil {
			return err
		}
	}
	return nil
}

func loadProjectedItems(layout Layout, payloadDir string) (map[string]projectedItem, error) {
	payloadDir = strings.TrimSpace(payloadDir)
	if payloadDir == "" {
		return map[string]projectedItem{}, nil
	}
	resources, err := DiscoverPayloadResources(payloadDir)
	if err != nil {
		return nil, err
	}
	items := make(map[string]projectedItem, len(resources.Rules)+len(resources.Skills))
	for _, rule := range resources.Rules {
		source := filepath.Join(payloadDir, filepath.FromSlash(rule.Path))
		dest := filepath.Join(layout.RulesRoot(), rule.Name+".md")
		if !pathWithin(layout.RulesRoot(), dest) {
			return nil, fmt.Errorf("plugin rule %s escapes rules root", rule.Name)
		}
		digest, err := fileSHA256(source)
		if err != nil {
			return nil, err
		}
		items[dest] = projectedItem{Kind: projectionRule, Name: rule.Name, Source: source, Destination: dest, Digest: digest}
	}
	for _, skill := range resources.Skills {
		source := filepath.Join(payloadDir, "skills", skill.Name)
		dest := filepath.Join(layout.SkillsRoot(), skill.Name)
		if !pathWithin(layout.SkillsRoot(), dest) {
			return nil, fmt.Errorf("plugin skill %s escapes skills root", skill.Name)
		}
		digest, err := payloadTreeSHA256(source)
		if err != nil {
			return nil, err
		}
		items[dest] = projectedItem{Kind: projectionSkill, Name: skill.Name, Source: source, Destination: dest, Digest: digest}
	}
	return items, nil
}

func preflightProjections(current, next map[string]projectedItem) error {
	for _, dest := range sortedDests(next) {
		item := next[dest]
		actual, exists, err := destinationDigest(item)
		if err != nil {
			return err
		}
		if !exists || actual == item.Digest {
			continue
		}
		if cur, ok := current[dest]; ok && actual == cur.Digest {
			continue
		}
		return fmt.Errorf("%w: %s %s", ErrProjectionConflict, item.Kind, item.Name)
	}
	for _, dest := range sortedDests(current) {
		if _, keep := next[dest]; keep {
			continue
		}
		item := current[dest]
		actual, exists, err := destinationDigest(item)
		if err != nil {
			return err
		}
		if !exists || actual == item.Digest {
			continue
		}
		return fmt.Errorf("%w: %s %s", ErrProjectionDrift, item.Kind, item.Name)
	}
	return nil
}

func projectItem(item projectedItem) error {
	switch item.Kind {
	case projectionRule:
		return replaceFileCopy(item.Source, item.Destination)
	case projectionSkill:
		return replaceDirCopy(item.Source, item.Destination)
	default:
		return fmt.Errorf("unknown plugin resource kind %q", item.Kind)
	}
}

func removeProjectedItem(item projectedItem) error {
	if item.Kind == projectionSkill {
		return os.RemoveAll(item.Destination)
	}
	if err := os.Remove(item.Destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func replaceFileCopy(source, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+"-*")
	if err != nil {
		return err
	}
	staging := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(staging)
		return err
	}
	_ = os.Remove(staging)
	if err := copyRegularFile(source, staging, info.Mode().Perm()); err != nil {
		_ = os.Remove(staging)
		return err
	}
	if err := os.Rename(staging, dest); err != nil {
		_ = os.Remove(staging)
		return err
	}
	return nil
}

func replaceDirCopy(source, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(dest), "."+filepath.Base(dest)+"-*")
	if err != nil {
		return err
	}
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := copyPayloadTree(source, staging); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	backup := ""
	if _, err := os.Lstat(dest); err == nil {
		backup = staging + ".old"
		_ = os.RemoveAll(backup)
		if err := os.Rename(dest, backup); err != nil {
			_ = os.RemoveAll(staging)
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = os.RemoveAll(staging)
		return err
	}
	if err := os.Rename(staging, dest); err != nil {
		if backup != "" {
			_ = os.Rename(backup, dest)
		}
		_ = os.RemoveAll(staging)
		return err
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func destinationDigest(item projectedItem) (string, bool, error) {
	info, err := os.Lstat(item.Destination)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", true, fmt.Errorf("%w: %s %s is a symlink", ErrProjectionConflict, item.Kind, item.Name)
	}
	switch item.Kind {
	case projectionRule:
		if !info.Mode().IsRegular() {
			return "", true, fmt.Errorf("%w: %s %s is not a regular file", ErrProjectionConflict, item.Kind, item.Name)
		}
		digest, err := fileSHA256(item.Destination)
		return digest, true, err
	case projectionSkill:
		if !info.IsDir() {
			return "", true, fmt.Errorf("%w: %s %s is not a directory", ErrProjectionConflict, item.Kind, item.Name)
		}
		digest, err := payloadTreeSHA256(item.Destination)
		return digest, true, err
	default:
		return "", true, fmt.Errorf("unknown plugin resource kind %q", item.Kind)
	}
}

func sortedDests(items map[string]projectedItem) []string {
	keys := make([]string, 0, len(items))
	for dest := range items {
		keys = append(keys, dest)
	}
	sort.Strings(keys)
	return keys
}
