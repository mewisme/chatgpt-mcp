package nativeinstruction

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

const (
	ScopeGlobal    = "global"
	ScopeWorkspace = "workspace"
	ModeCreate     = "create"
	ModeUpdate     = "update"

	maxFileBytes = 512 * 1024
	maxTreeBytes = 2 * 1024 * 1024
)

type File struct {
	Path       string
	Data       []byte
	Executable bool
}

type SkillRequest struct {
	Scope         string
	Mode          string
	Name          string
	Description   string
	Instructions  string
	WorkspaceRoot string
	Files         []File
	Remove        []string
	DryRun        bool
}

type SkillResult struct {
	Scope        string   `json:"scope"`
	Mode         string   `json:"mode"`
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Directory    string   `json:"directory"`
	FilesWritten []string `json:"files_written"`
	FilesRemoved []string `json:"files_removed"`
	SHA256       string   `json:"sha256"`
	DryRun       bool     `json:"dry_run"`
}

type RuleRequest struct {
	Scope         string
	Mode          string
	Name          string
	Description   string
	AlwaysApply   bool
	Globs         []string
	Content       string
	WorkspaceRoot string
	DryRun        bool
}

type RuleResult struct {
	Scope  string `json:"scope"`
	Mode   string `json:"mode"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	DryRun bool   `json:"dry_run"`
}

func WriteSkill(req SkillRequest) (SkillResult, error) {
	scope, mode, name, err := normalizeIdentity(req.Scope, req.Mode, req.Name)
	if err != nil {
		return SkillResult{}, err
	}
	description := strings.TrimSpace(req.Description)
	instructions := strings.TrimSpace(req.Instructions)
	if description == "" {
		return SkillResult{}, errors.New("description is required")
	}
	if instructions == "" {
		return SkillResult{}, errors.New("instructions are required")
	}
	layout, err := resolveLayout(scope, req.WorkspaceRoot)
	if err != nil {
		return SkillResult{}, err
	}
	dir := filepath.Join(layout.SkillsRoot(), name)
	if err := rejectOwned(layout, dir); err != nil {
		return SkillResult{}, err
	}
	if err := checkMode(mode, dir, true); err != nil {
		return SkillResult{}, err
	}
	files, removed, err := planSkillTree(dir, mode, name, description, instructions, req.Files, req.Remove)
	if err != nil {
		return SkillResult{}, err
	}
	digest := hashPlanned(files)
	written := make([]string, 0, len(files))
	for rel := range files {
		written = append(written, filepath.Join(dir, filepath.FromSlash(rel)))
	}
	sort.Strings(written)
	sort.Strings(removed)
	result := SkillResult{Scope: scope, Mode: mode, Name: name, Path: filepath.Join(dir, "SKILL.md"), Directory: dir, FilesWritten: written, FilesRemoved: removed, SHA256: digest, DryRun: req.DryRun}
	if req.DryRun {
		return result, nil
	}
	if err := writeSkillTree(dir, files); err != nil {
		return SkillResult{}, err
	}
	return result, nil
}

func WriteRule(req RuleRequest) (RuleResult, error) {
	scope, mode, name, err := normalizeIdentity(req.Scope, req.Mode, req.Name)
	if err != nil {
		return RuleResult{}, err
	}
	description := strings.TrimSpace(req.Description)
	content := strings.TrimSpace(req.Content)
	if description == "" {
		return RuleResult{}, errors.New("description is required")
	}
	if content == "" {
		return RuleResult{}, errors.New("content is required")
	}
	globs, err := normalizeGlobs(req.AlwaysApply, req.Globs)
	if err != nil {
		return RuleResult{}, err
	}
	layout, err := resolveLayout(scope, req.WorkspaceRoot)
	if err != nil {
		return RuleResult{}, err
	}
	path := filepath.Join(layout.RulesRoot(), name+".md")
	if err := rejectOwned(layout, path); err != nil {
		return RuleResult{}, err
	}
	if err := checkMode(mode, path, false); err != nil {
		return RuleResult{}, err
	}
	body := renderRule(description, req.AlwaysApply, globs, content)
	digest := sha256Hex(body)
	result := RuleResult{Scope: scope, Mode: mode, Name: name, Path: path, SHA256: digest, DryRun: req.DryRun}
	if req.DryRun {
		return result, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return RuleResult{}, err
	}
	if err := state.WriteFileAtomic(path, []byte(body), 0600); err != nil {
		return RuleResult{}, err
	}
	return result, nil
}

func normalizeIdentity(scope, mode, name string) (string, string, string, error) {
	scope = strings.TrimSpace(scope)
	switch scope {
	case ScopeGlobal, ScopeWorkspace:
	default:
		return "", "", "", fmt.Errorf("scope must be %s or %s", ScopeGlobal, ScopeWorkspace)
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = ModeCreate
	}
	switch mode {
	case ModeCreate, ModeUpdate:
	default:
		return "", "", "", errors.New("mode must be create or update")
	}
	name = strings.TrimSpace(name)
	if !plugin.ValidCanonicalName(name) {
		return "", "", "", fmt.Errorf("invalid resource name %q", name)
	}
	return scope, mode, name, nil
}

func resolveLayout(scope, workspaceRoot string) (plugin.Layout, error) {
	switch scope {
	case ScopeGlobal:
		layout := plugin.DefaultLayout()
		return layout, layout.Validate()
	case ScopeWorkspace:
		return plugin.WorkspaceLayout(workspaceRoot)
	default:
		return plugin.Layout{}, fmt.Errorf("scope must be %s or %s", ScopeGlobal, ScopeWorkspace)
	}
}

func rejectOwned(layout plugin.Layout, dest string) error {
	id, owned, err := plugin.ProjectedDestinationOwner(layout, dest)
	if err != nil {
		return err
	}
	if owned {
		return fmt.Errorf("%w: owned by plugin %s", plugin.ErrProjectionConflict, id)
	}
	return nil
}

func checkMode(mode, dest string, dir bool) error {
	info, err := os.Lstat(dest)
	if errors.Is(err, os.ErrNotExist) {
		if mode == ModeUpdate {
			return fmt.Errorf("%s does not exist", dest)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink", dest)
	}
	if dir && !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dest)
	}
	if !dir && !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", dest)
	}
	if mode == ModeCreate {
		return fmt.Errorf("%s already exists", dest)
	}
	return nil
}

func planSkillTree(dir, mode, name, description, instructions string, upserts []File, remove []string) (map[string]plannedFile, []string, error) {
	files := map[string]plannedFile{}
	if mode == ModeUpdate {
		existing, err := loadSkillTree(dir)
		if err != nil {
			return nil, nil, err
		}
		files = existing
	}
	removedRels, err := normalizeRemove(remove)
	if err != nil {
		return nil, nil, err
	}
	upserted := map[string]bool{}
	total := 0
	for _, item := range upserts {
		rel, err := cleanSkillRel(item.Path)
		if err != nil {
			return nil, nil, err
		}
		if len(item.Data) > maxFileBytes {
			return nil, nil, fmt.Errorf("supporting file %s exceeds %d bytes", rel, maxFileBytes)
		}
		if upserted[rel] {
			return nil, nil, fmt.Errorf("duplicate supporting file %s", rel)
		}
		upserted[rel] = true
		files[rel] = plannedFile{Data: append([]byte(nil), item.Data...), Mode: fileMode(item.Executable)}
	}
	for _, rel := range removedRels {
		if upserted[rel] {
			return nil, nil, fmt.Errorf("remove_files path %s also appears in files", rel)
		}
		if _, ok := files[rel]; !ok {
			continue
		}
		delete(files, rel)
	}
	files["SKILL.md"] = plannedFile{Data: []byte(renderSkill(name, description, instructions)), Mode: 0600}
	removed := make([]string, 0, len(removedRels))
	for _, rel := range removedRels {
		removed = append(removed, filepath.Join(dir, filepath.FromSlash(rel)))
	}
	for _, item := range files {
		total += len(item.Data)
		if total > maxTreeBytes {
			return nil, nil, fmt.Errorf("skill tree exceeds %d bytes", maxTreeBytes)
		}
	}
	return files, removed, nil
}

type plannedFile struct {
	Data []byte
	Mode os.FileMode
}

func loadSkillTree(dir string) (map[string]plannedFile, error) {
	files := map[string]plannedFile{}
	err := filepath.WalkDir(dir, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink", current)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", current)
		}
		relative, err := filepath.Rel(dir, current)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relative)
		if rel == "SKILL.md" {
			return nil
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		files[rel] = plannedFile{Data: data, Mode: fileMode(info.Mode().Perm()&0111 != 0)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func writeSkillTree(dir string, files map[string]plannedFile) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(dir), "."+filepath.Base(dir)+"-*")
	if err != nil {
		return err
	}
	if err := materialize(staging, files); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	backup := ""
	if _, err := os.Lstat(dir); err == nil {
		backup = staging + ".old"
		_ = os.RemoveAll(backup)
		if err := os.Rename(dir, backup); err != nil {
			_ = os.RemoveAll(staging)
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = os.RemoveAll(staging)
		return err
	}
	if err := os.Rename(staging, dir); err != nil {
		if backup != "" {
			_ = os.Rename(backup, dir)
		}
		_ = os.RemoveAll(staging)
		return err
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func materialize(root string, files map[string]plannedFile) error {
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		item := files[rel]
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(path, item.Data, item.Mode); err != nil {
			return err
		}
	}
	return nil
}

func hashPlanned(files map[string]plannedFile) string {
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	hash := sha256.New()
	for _, rel := range rels {
		item := files[rel]
		_, _ = fmt.Fprintf(hash, "file\x00%s\x00%04o\x00", rel, item.Mode.Perm())
		_, _ = hash.Write(item.Data)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func fileMode(executable bool) os.FileMode {
	if executable {
		return 0755
	}
	return 0600
}

func cleanSkillRel(path string) (string, error) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" || path == "SKILL.md" || strings.EqualFold(path, "skill.md") {
		return "", errors.New("supporting path cannot target SKILL.md")
	}
	if filepath.IsAbs(path) || strings.Contains(path, `\`) {
		return "", fmt.Errorf("supporting path must be a clean relative path: %s", path)
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("supporting path escapes the skill directory: %s", path)
		}
	}
	return path, nil
}

func normalizeRemove(paths []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := cleanSkillRel(path)
		if err != nil {
			return nil, err
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		result = append(result, rel)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeGlobs(alwaysApply bool, globs []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(globs))
	for _, glob := range globs {
		value := strings.TrimSpace(glob)
		if value == "" {
			return nil, errors.New("globs must not contain empty entries")
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	if alwaysApply && len(result) > 0 {
		return nil, errors.New("always_apply=true requires empty globs")
	}
	if !alwaysApply && len(result) == 0 {
		return nil, errors.New("always_apply=false requires at least one glob")
	}
	return result, nil
}

func renderSkill(name, description, instructions string) string {
	return "---\nname: " + name + "\ndescription: " + yamlScalar(description) + "\n---\n\n" + strings.TrimSpace(instructions) + "\n"
}

func renderRule(description string, alwaysApply bool, globs []string, content string) string {
	var b strings.Builder
	b.WriteString("---\ndescription: ")
	b.WriteString(yamlScalar(description))
	b.WriteByte('\n')
	if alwaysApply {
		b.WriteString("alwaysApply: true\n")
	} else {
		b.WriteString("globs:\n")
		for _, glob := range globs {
			b.WriteString("  - ")
			b.WriteString(strconv.Quote(glob))
			b.WriteByte('\n')
		}
		b.WriteString("alwaysApply: false\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(content))
	b.WriteByte('\n')
	return b.String()
}

func yamlScalar(value string) string {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, ":#{}[]&*!|>'\"%@`\n\t") {
		return strconv.Quote(value)
	}
	return value
}
