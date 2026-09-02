package instructioncontext

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultSectionMaxBytes = 25_000
	DefaultSectionMaxLines = 200
)

type MemoryLoadOptions struct {
	WorkspaceRoots     []string
	HomeDir            string
	DisableUser        bool
	MaxBytesPerSection int
	MaxLinesPerSection int
	ImportMaxDepth     int
	Now                func() time.Time
}

type memoryCandidate struct {
	Relative string
	Kind     SectionKind
	Source   string
}

var projectMemoryCandidates = []memoryCandidate{
	{Relative: "AGENTS.md", Kind: SectionProject, Source: "agents"},
	{Relative: filepath.Join(".agents", "AGENTS.md"), Kind: SectionProject, Source: "agents"},
	{Relative: "CLAUDE.md", Kind: SectionProject, Source: "claude"},
	{Relative: filepath.Join(".claude", "CLAUDE.md"), Kind: SectionProject, Source: "claude"},
	{Relative: filepath.Join(".claudes", "CLAUDE.md"), Kind: SectionProject, Source: "claudes"},
	{Relative: filepath.Join(".cursor", "AGENTS.md"), Kind: SectionProject, Source: "cursor"},
	{Relative: filepath.Join(".codex", "AGENTS.md"), Kind: SectionProject, Source: "codex"},
	{Relative: "CLAUDE.local.md", Kind: SectionProject, Source: "claude"},
}

var userMemoryCandidates = []memoryCandidate{
	{Relative: filepath.Join(".agents", "AGENTS.md"), Kind: SectionUser, Source: "agents"},
	{Relative: filepath.Join(".claude", "CLAUDE.md"), Kind: SectionUser, Source: "claude"},
	{Relative: filepath.Join(".claudes", "CLAUDE.md"), Kind: SectionUser, Source: "claudes"},
	{Relative: filepath.Join(".cursor", "AGENTS.md"), Kind: SectionUser, Source: "cursor"},
	{Relative: filepath.Join(".codex", "AGENTS.md"), Kind: SectionUser, Source: "codex"},
}

func LoadProjectMemory(root string, opts MemoryLoadOptions) (ProjectMemoryBundle, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return ProjectMemoryBundle{}, err
	}
	root = filepath.Clean(absolute)
	workspaceRoots := opts.WorkspaceRoots
	if len(workspaceRoots) == 0 {
		workspaceRoots = []string{root}
	} else {
		workspaceRoots = cleanPaths(workspaceRoots)
	}
	maxBytes := opts.MaxBytesPerSection
	if maxBytes <= 0 {
		maxBytes = DefaultSectionMaxBytes
	}
	maxLines := opts.MaxLinesPerSection
	if maxLines <= 0 {
		maxLines = DefaultSectionMaxLines
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	home := strings.TrimSpace(opts.HomeDir)
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	expander := newImportExpander(workspaceRoots, home, opts.ImportMaxDepth, maxBytes, maxLines)
	sections := make([]Section, 0, len(projectMemoryCandidates)+len(userMemoryCandidates))
	totalBytes := 0
	seenContent := map[string]bool{}
	appendCandidate := func(base string, candidate memoryCandidate) {
		section, contentID, ok := loadMemorySection(filepath.Join(base, candidate.Relative), candidate, maxBytes, maxLines, expander)
		if !ok || seenContent[contentID] {
			return
		}
		seenContent[contentID] = true
		sections = append(sections, section)
		totalBytes += section.LoadedBytes
	}
	if !opts.DisableUser {
		for _, candidate := range userMemoryCandidates {
			appendCandidate(home, candidate)
		}
	}
	for _, candidate := range projectMemoryCandidates {
		appendCandidate(root, candidate)
	}
	return ProjectMemoryBundle{Root: root, WorkspaceRoots: workspaceRoots, Sections: sections, Imports: expander.imports, TotalBytes: totalBytes, LoadedAt: now().UTC()}, nil
}

func loadMemorySection(path string, candidate memoryCandidate, maxBytes, maxLines int, expander *importExpander) (Section, string, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Section{}, "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Section{}, "", false
	}
	expanded := expander.expand(string(data), path)
	content, truncated := limitInstructionText([]byte(expanded), maxBytes, maxLines)
	content = strings.TrimSpace(content)
	if content == "" {
		return Section{}, "", false
	}
	loadedBytes := len([]byte(content))
	normalized := strings.TrimSpace(strings.ReplaceAll(stripHTMLComments(expanded), "\r\n", "\n"))
	sum := sha256.Sum256([]byte(normalized))
	return Section{
		Path: path, Kind: candidate.Kind, Source: candidate.Source, Content: content,
		Truncated: truncated, OriginalBytes: len(data), LoadedBytes: loadedBytes,
	}, hex.EncodeToString(sum[:]), true
}

func limitInstructionText(data []byte, maxBytes, maxLines int) (string, bool) {
	original := string(data)
	lines := strings.Split(strings.ReplaceAll(original, "\r\n", "\n"), "\n")
	truncated := false
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}
	limited := []byte(strings.Join(lines, "\n"))
	if len(limited) > maxBytes {
		limited = limited[:maxBytes]
		for len(limited) > 0 && !utf8.Valid(limited) {
			limited = limited[:len(limited)-1]
		}
		truncated = true
	}
	return string(limited), truncated
}

func cleanPaths(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		absolute, err := filepath.Abs(value)
		if err != nil {
			continue
		}
		result = append(result, filepath.Clean(absolute))
	}
	return result
}
