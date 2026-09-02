package instructioncontext

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const DefaultImportMaxDepth = 4

var htmlCommentPattern = regexp.MustCompile(`(?s)<!--.*?-->`)

type importExpander struct {
	roots      []string
	home       string
	maxDepth   int
	maxBytes   int
	maxLines   int
	imports    []Section
	importSeen map[string]bool
}

func newImportExpander(roots []string, home string, maxDepth, maxBytes, maxLines int) *importExpander {
	if maxDepth <= 0 {
		maxDepth = DefaultImportMaxDepth
	}
	return &importExpander{roots: cleanPaths(roots), home: filepath.Clean(home), maxDepth: maxDepth, maxBytes: maxBytes, maxLines: maxLines, importSeen: map[string]bool{}}
}

func (e *importExpander) expand(content, sourcePath string) string {
	visited := map[string]bool{}
	if canonical, err := canonicalEnvironmentPath(sourcePath); err == nil {
		visited[canonical] = true
		sourcePath = canonical
	}
	return e.expandDepth(stripHTMLComments(content), filepath.Dir(sourcePath), visited, 0)
}

func (e *importExpander) expandDepth(content, baseDir string, visited map[string]bool, depth int) string {
	if depth >= e.maxDepth {
		return content
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			out = append(out, line)
			continue
		}
		if inFence {
			out = append(out, line)
			continue
		}
		importPath, ok := parseImportLine(trimmed)
		if !ok {
			out = append(out, line)
			continue
		}
		resolved, err := e.resolveImport(importPath, baseDir)
		if err != nil {
			out = append(out, fmt.Sprintf("<!-- import denied: %s (%v) -->", importPath, err))
			continue
		}
		if visited[resolved] {
			out = append(out, fmt.Sprintf("<!-- skipped circular import %s -->", resolved))
			continue
		}
		info, err := os.Lstat(resolved)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			out = append(out, fmt.Sprintf("<!-- import failed: %s -->", resolved))
			continue
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			out = append(out, fmt.Sprintf("<!-- import failed: %s -->", resolved))
			continue
		}
		visited[resolved] = true
		expanded := e.expandDepth(stripHTMLComments(string(data)), filepath.Dir(resolved), visited, depth+1)
		delete(visited, resolved)
		limited, truncated := limitInstructionText([]byte(expanded), e.maxBytes, e.maxLines)
		limited = strings.TrimSpace(limited)
		if limited == "" {
			continue
		}
		if !e.importSeen[resolved] {
			e.importSeen[resolved] = true
			e.imports = append(e.imports, Section{Path: resolved, Kind: SectionImport, Source: "import", Content: limited, Truncated: truncated, OriginalBytes: len(data), LoadedBytes: len([]byte(limited))})
		}
		out = append(out, fmt.Sprintf("<!-- @import %s -->", resolved), limited)
	}
	return strings.Join(out, "\n")
}

func parseImportLine(line string) (string, bool) {
	if len(line) < 2 || line[0] != '@' {
		return "", false
	}
	value := strings.TrimSpace(line[1:])
	if value == "" || strings.ContainsAny(value, " \t\r\n") || strings.ContainsRune(value, rune(96)) {
		return "", false
	}
	return value, true
}

func (e *importExpander) resolveImport(value, baseDir string) (string, error) {
	var target string
	switch {
	case strings.HasPrefix(value, "~/"):
		if e.home == "" || e.home == "." {
			return "", fmt.Errorf("home directory unavailable")
		}
		target = filepath.Join(e.home, strings.TrimPrefix(value, "~/"))
	case filepath.IsAbs(value):
		target = value
	default:
		target = filepath.Join(baseDir, value)
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	resolved := filepath.Clean(absolute)
	if info, err := os.Lstat(resolved); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("symlink imports are not allowed")
	}
	if canonical, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = filepath.Clean(canonical)
	}
	if !withinAnyRoot(e.roots, resolved) {
		return "", fmt.Errorf("outside effective workspace roots")
	}
	return resolved, nil
}

func stripHTMLComments(content string) string {
	return htmlCommentPattern.ReplaceAllString(content, "")
}

func withinAnyRoot(roots []string, target string) bool {
	for _, root := range roots {
		rel, err := filepath.Rel(root, target)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)) {
			return true
		}
	}
	return false
}
