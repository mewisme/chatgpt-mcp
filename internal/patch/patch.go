package patch

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type HunkLine struct {
	Type string
	Text string
}

type Hunk struct {
	OldStart int
	HasLine  bool
	Lines    []HunkLine
}

type MultiFileOp struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Patch     string `json:"patch,omitempty"`
	Content   string `json:"content,omitempty"`
}

type MultiPatchResult struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	OK        bool   `json:"ok"`
	Diff      string `json:"diff,omitempty"`
	Error     string `json:"error,omitempty"`
}

var standardHunkHeader = regexp.MustCompile(`^@@\s*-(\d+)(?:,\d+)?\s+\+\d+(?:,\d+)?\s+@@`)

func ApplyUnifiedPatchToText(original, patchText string) (string, error) {
	eol := detectEOL(original)
	output := strings.Split(normalizeEOL(original), "\n")
	hunks, err := parsePatch(patchText)
	if err != nil {
		return "", err
	}
	delta := 0
	searchFrom := 0
	for _, hunk := range hunks {
		if hunk.HasLine {
			index := hunk.OldStart - 1 + delta
			if index < 0 || index > len(output) {
				return "", fmt.Errorf("patch hunk line %d is outside file", hunk.OldStart)
			}
			removeCount := hunkRemoveCount(hunk)
			if index+removeCount > len(output) {
				return "", fmt.Errorf("patch hunk at line %d exceeds file", hunk.OldStart)
			}
			replacement := hunkReplacement(hunk)
			output = append(output[:index], append(replacement, output[index+removeCount:]...)...)
			delta += len(replacement) - removeCount
			continue
		}
		pattern := hunkSearchPattern(hunk)
		index := findPatternIndex(output, pattern, searchFrom)
		if index < 0 {
			preview := strings.Join(firstStrings(pattern, 3), " | ")
			if preview == "" {
				preview = "(empty hunk)"
			}
			return "", fmt.Errorf("patch context not found in file. Expected lines like: %s", preview)
		}
		replacement := hunkReplacement(hunk)
		removeCount := hunkRemoveCount(hunk)
		output = append(output[:index], append(replacement, output[index+removeCount:]...)...)
		searchFrom = index + len(replacement)
	}
	result := strings.Join(output, "\n")
	if eol == "\r\n" {
		result = strings.ReplaceAll(result, "\n", "\r\n")
	}
	return result, nil
}

func IsMultiFilePatch(patchText string) bool {
	text := strings.TrimSpace(patchText)
	return strings.Contains(text, "*** Begin Patch") ||
		strings.Contains(text, "*** Update File:") ||
		strings.Contains(text, "*** Add File:") ||
		strings.Contains(text, "*** Delete File:") ||
		regexp.MustCompile(`(?m)^---\s+`).MatchString(text) ||
		regexp.MustCompile(`(?m)^\+\+\+\s+`).MatchString(text)
}

func ParseMultiFilePatch(patchText, baseDir string) ([]MultiFileOp, error) {
	normalized := normalizeEOL(strings.TrimSpace(patchText))
	if strings.Contains(normalized, "*** Begin Patch") || strings.Contains(normalized, "*** Update File:") || strings.Contains(normalized, "*** Add File:") || strings.Contains(normalized, "*** Delete File:") {
		return parseCodexMultiFilePatch(normalized, baseDir)
	}
	return parseUnifiedMultiFilePatch(normalized, baseDir)
}

func BuildSimpleDiff(oldContent, newContent string) string {
	oldLines := strings.Split(normalizeEOL(oldContent), "\n")
	newLines := strings.Split(normalizeEOL(newContent), "\n")
	var diff []string
	max := len(oldLines)
	if len(newLines) > max {
		max = len(newLines)
	}
	for i := 0; i < max; i++ {
		var oldLine, newLine *string
		if i < len(oldLines) {
			value := oldLines[i]
			oldLine = &value
		}
		if i < len(newLines) {
			value := newLines[i]
			newLine = &value
		}
		if oldLine != nil && newLine != nil && *oldLine == *newLine {
			continue
		}
		if oldLine != nil {
			diff = append(diff, "- "+*oldLine)
		}
		if newLine != nil {
			diff = append(diff, "+ "+*newLine)
		}
	}
	if len(diff) == 0 {
		return "(no visible diff)"
	}
	return strings.Join(diff, "\n")
}

func parsePatch(patchText string) ([]Hunk, error) {
	normalized := normalizeEOL(strings.TrimSpace(patchText))
	rawLines := strings.Split(normalized, "\n")
	hunks := make([]Hunk, 0)
	for i := 0; i < len(rawLines); {
		line := rawLines[i]
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "diff ") {
			i++
			continue
		}
		if !strings.HasPrefix(line, "@@") {
			i++
			continue
		}
		hunk := Hunk{}
		if match := standardHunkHeader.FindStringSubmatch(line); len(match) > 1 {
			value, err := strconv.Atoi(match[1])
			if err != nil {
				return nil, err
			}
			hunk.OldStart = value
			hunk.HasLine = true
		}
		i++
		for ; i < len(rawLines); i++ {
			patchLine := rawLines[i]
			if strings.HasPrefix(patchLine, "@@") {
				break
			}
			if patchLine == `\ No newline at end of file` {
				continue
			}
			switch {
			case strings.HasPrefix(patchLine, " "):
				hunk.Lines = append(hunk.Lines, HunkLine{Type: "context", Text: patchLine[1:]})
			case strings.HasPrefix(patchLine, "-"):
				hunk.Lines = append(hunk.Lines, HunkLine{Type: "remove", Text: patchLine[1:]})
			case strings.HasPrefix(patchLine, "+"):
				hunk.Lines = append(hunk.Lines, HunkLine{Type: "add", Text: patchLine[1:]})
			case patchLine == "":
				hunk.Lines = append(hunk.Lines, HunkLine{Type: "context", Text: ""})
			}
		}
		if len(hunk.Lines) > 0 {
			hunks = append(hunks, hunk)
		}
	}
	if len(hunks) == 0 {
		return nil, errors.New("no valid patch hunks found. Use @@ header with +/- lines")
	}
	return hunks, nil
}

func parseCodexMultiFilePatch(normalized, baseDir string) ([]MultiFileOp, error) {
	lines := strings.Split(normalized, "\n")
	ops := make([]MultiFileOp, 0)
	var current *MultiFileOp
	chunk := make([]string, 0)
	flush := func() {
		if current == nil {
			return
		}
		if current.Operation == "create" {
			added := make([]string, 0, len(chunk))
			for _, line := range chunk {
				if strings.HasPrefix(line, "+") {
					added = append(added, line[1:])
				}
			}
			current.Content = strings.Join(added, "\n")
		} else if current.Operation == "update" {
			current.Patch = strings.Join(chunk, "\n")
		}
		ops = append(ops, *current)
		current = nil
		chunk = chunk[:0]
	}
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "*** Update File:"):
			flush()
			current = &MultiFileOp{Path: resolvePatchPath(strings.TrimSpace(strings.TrimPrefix(line, "*** Update File:")), baseDir), Operation: "update"}
		case strings.HasPrefix(line, "*** Add File:"):
			flush()
			current = &MultiFileOp{Path: resolvePatchPath(strings.TrimSpace(strings.TrimPrefix(line, "*** Add File:")), baseDir), Operation: "create"}
		case strings.HasPrefix(line, "*** Delete File:"):
			flush()
			ops = append(ops, MultiFileOp{Path: resolvePatchPath(strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File:")), baseDir), Operation: "delete"})
		case strings.HasPrefix(line, "*** Begin Patch"), strings.HasPrefix(line, "*** End Patch"):
		default:
			if current != nil {
				chunk = append(chunk, line)
			}
		}
	}
	flush()
	if len(ops) == 0 {
		return nil, errors.New("no file operations found in patch")
	}
	return ops, nil
}

func parseUnifiedMultiFilePatch(normalized, baseDir string) ([]MultiFileOp, error) {
	lines := strings.Split(normalized, "\n")
	ops := make([]MultiFileOp, 0)
	for i := 0; i < len(lines); {
		if !strings.HasPrefix(lines[i], "--- ") {
			i++
			continue
		}
		oldPath := cleanDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[i], "--- ")))
		if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "+++ ") {
			return nil, errors.New("unified diff is missing +++ header")
		}
		newPath := cleanDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[i+1], "+++ ")))
		i += 2
		start := i
		for i < len(lines) && !strings.HasPrefix(lines[i], "--- ") {
			i++
		}
		block := strings.Join(lines[start:i], "\n")
		switch {
		case oldPath == "/dev/null":
			ops = append(ops, MultiFileOp{Path: resolvePatchPath(newPath, baseDir), Operation: "create", Content: addedContentFromPatch(block)})
		case newPath == "/dev/null":
			ops = append(ops, MultiFileOp{Path: resolvePatchPath(oldPath, baseDir), Operation: "delete"})
		default:
			ops = append(ops, MultiFileOp{Path: resolvePatchPath(oldPath, baseDir), Operation: "update", Patch: block})
		}
	}
	if len(ops) == 0 {
		return nil, errors.New("no file operations found in patch")
	}
	return ops, nil
}

func addedContentFromPatch(block string) string {
	var added []string
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added = append(added, line[1:])
		}
	}
	return strings.Join(added, "\n")
}

func cleanDiffPath(value string) string {
	if strings.HasPrefix(value, "a/") || strings.HasPrefix(value, "b/") {
		return value[2:]
	}
	return value
}

func resolvePatchPath(input, baseDir string) string {
	cleaned := strings.Trim(strings.TrimSpace(input), `"'`)
	if filepath.IsAbs(cleaned) {
		return filepath.Clean(cleaned)
	}
	if baseDir != "" {
		return filepath.Join(baseDir, cleaned)
	}
	return filepath.Clean(cleaned)
}

func hunkSearchPattern(hunk Hunk) []string {
	result := make([]string, 0, len(hunk.Lines))
	for _, line := range hunk.Lines {
		if line.Type == "context" || line.Type == "remove" {
			result = append(result, line.Text)
		}
	}
	return result
}

func hunkReplacement(hunk Hunk) []string {
	result := make([]string, 0, len(hunk.Lines))
	for _, line := range hunk.Lines {
		if line.Type == "context" || line.Type == "add" {
			result = append(result, line.Text)
		}
	}
	return result
}

func hunkRemoveCount(hunk Hunk) int {
	count := 0
	for _, line := range hunk.Lines {
		if line.Type == "context" || line.Type == "remove" {
			count++
		}
	}
	return count
}

func findPatternIndex(haystack, needle []string, startAt int) int {
	if len(needle) == 0 {
		return startAt
	}
	for i := startAt; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func firstStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}

func normalizeEOL(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func detectEOL(text string) string {
	if strings.Contains(text, "\r\n") {
		return "\r\n"
	}
	return "\n"
}
