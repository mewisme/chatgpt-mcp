package tools

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type GlobMatch struct {
	Path    string
	ModTime time.Time
}

type GrepOptions struct {
	Pattern         string
	Path            string
	Glob            string
	OutputMode      string
	CaseInsensitive bool
	Multiline       bool
	HeadLimit       int
	ContextBefore   int
	ContextAfter    int
	ContextAround   int
}

type TreeNode struct {
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Truncated bool       `json:"truncated,omitempty"`
	Children  []TreeNode `json:"children,omitempty"`
}

func globFiles(rootDir *rootedDirectory, pattern string, maxResults int) ([]GlobMatch, error) {
	matcher, err := globRegexp(pattern)
	if err != nil {
		return nil, err
	}
	matches := make([]GlobMatch, 0)
	var walk func(*rootedDirectory, string)
	walk = func(dir *rootedDirectory, relativeBase string) {
		if len(matches) >= maxResults {
			return
		}
		entries, err := dir.ReadDir()
		if err != nil {
			return
		}
		for _, entry := range entries {
			if len(matches) >= maxResults {
				break
			}
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			relative := filepath.Join(relativeBase, entry.Name())
			fullPath := filepath.Join(rootDir.absolute, relative)
			if entry.IsDir() {
				if !skipGlobDirectory(entry.Name()) {
					child, err := dir.OpenChild(entry.Name())
					if err == nil {
						walk(child, relative)
						_ = child.Close()
					}
				}
				continue
			}
			if !matcher.MatchString(filepath.ToSlash(relative)) && !matcher.MatchString(entry.Name()) {
				continue
			}
			info, err := entry.Info()
			if err == nil {
				matches = append(matches, GlobMatch{Path: fullPath, ModTime: info.ModTime()})
			}
		}
	}
	walk(rootDir, "")
	sort.Slice(matches, func(i, j int) bool { return matches[i].ModTime.After(matches[j].ModTime) })
	return matches, nil
}

func grepSearch(root *rootedDirectory, options GrepOptions) (string, error) {
	flags := ""
	if options.CaseInsensitive {
		flags += "i"
	}
	if options.Multiline {
		flags += "m"
	}
	pattern := options.Pattern
	if flags != "" {
		pattern = "(?" + flags + ")" + pattern
	}
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	globMatcher, err := simpleGlobRegexp(options.Glob)
	if err != nil {
		return "", err
	}
	before := options.ContextBefore
	after := options.ContextAfter
	if options.ContextAround > 0 {
		before = options.ContextAround
		after = options.ContextAround
	}
	fileMatches := map[string]int{}
	contentLines := make([]string, 0)
	var walk func(*rootedDirectory)
	walk = func(dir *rootedDirectory) {
		if options.OutputMode == "content" && len(contentLines) >= options.HeadLimit {
			return
		}
		entries, err := dir.ReadDir()
		if err != nil {
			return
		}
		for _, entry := range entries {
			if options.OutputMode == "content" && len(contentLines) >= options.HeadLimit {
				break
			}
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			fullPath := filepath.Join(dir.absolute, entry.Name())
			if entry.IsDir() {
				if entry.Name() != "node_modules" && entry.Name() != ".git" {
					child, err := dir.OpenChild(entry.Name())
					if err == nil {
						walk(child)
						_ = child.Close()
					}
				}
				continue
			}
			if !globMatcher.MatchString(entry.Name()) {
				continue
			}
			data, err := dir.ReadRegularFileLimited(entry.Name(), maxSearchFileBytes, "search file")
			if err != nil {
				continue
			}
			text := string(data)
			if options.Multiline {
				if regex.MatchString(text) {
					fileMatches[fullPath]++
					if options.OutputMode == "content" {
						contentLines = append(contentLines, fmt.Sprintf("%s: [multiline match x1]", fullPath))
					}
				}
				continue
			}
			lines := strings.Split(text, "\n")
			for index, line := range lines {
				if !regex.MatchString(line) {
					continue
				}
				fileMatches[fullPath]++
				if options.OutputMode != "content" {
					continue
				}
				if len(contentLines) >= options.HeadLimit {
					break
				}
				if before > 0 || after > 0 {
					start := index - before
					if start < 0 {
						start = 0
					}
					end := index + after
					if end >= len(lines) {
						end = len(lines) - 1
					}
					for lineIndex := start; lineIndex <= end && len(contentLines) < options.HeadLimit; lineIndex++ {
						prefix := "-"
						if lineIndex == index {
							prefix = ":"
						}
						contentLines = append(contentLines, fmt.Sprintf("%s%s%d: %s", fullPath, prefix, lineIndex+1, lines[lineIndex]))
					}
				} else {
					contentLines = append(contentLines, fmt.Sprintf("%s:%d: %s", fullPath, index+1, strings.TrimSpace(line)))
				}
			}
		}
	}
	walk(root)

	files := make([]string, 0, len(fileMatches))
	for file := range fileMatches {
		files = append(files, file)
	}
	sort.Strings(files)
	if len(files) > options.HeadLimit {
		files = files[:options.HeadLimit]
	}
	switch options.OutputMode {
	case "files_with_matches":
		if len(files) == 0 {
			return "No matches found", nil
		}
		return strings.Join(files, "\n"), nil
	case "count":
		if len(files) == 0 {
			return "No matches found", nil
		}
		rows := make([]string, len(files))
		for i, file := range files {
			rows[i] = fmt.Sprintf("%s:%d", file, fileMatches[file])
		}
		return strings.Join(rows, "\n"), nil
	default:
		if len(contentLines) == 0 {
			return "No matches found", nil
		}
		return strings.Join(contentLines, "\n"), nil
	}
}

func searchDirectory(root *rootedDirectory, regex *regexp.Regexp, globPattern string, maxResults int) []string {
	matcher, err := simpleGlobRegexp(globPattern)
	if err != nil {
		return []string{}
	}
	results := make([]string, 0)
	var walk func(*rootedDirectory)
	walk = func(dir *rootedDirectory) {
		if len(results) >= maxResults {
			return
		}
		entries, err := dir.ReadDir()
		if err != nil {
			return
		}
		for _, entry := range entries {
			if len(results) >= maxResults {
				break
			}
			if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
				continue
			}
			fullPath := filepath.Join(dir.absolute, entry.Name())
			if entry.IsDir() {
				child, err := dir.OpenChild(entry.Name())
				if err == nil {
					walk(child)
					_ = child.Close()
				}
				continue
			}
			if !matcher.MatchString(entry.Name()) {
				continue
			}
			data, err := dir.ReadRegularFileLimited(entry.Name(), maxSearchFileBytes, "search file")
			if err != nil {
				continue
			}
			for index, line := range strings.Split(string(data), "\n") {
				if len(results) >= maxResults {
					break
				}
				if regex.MatchString(line) {
					results = append(results, fmt.Sprintf("%s:%d: %s", fullPath, index+1, strings.TrimSpace(line)))
				}
			}
		}
	}
	walk(root)
	return results
}

func buildTree(path *rootedDirectory, depth, maxDepth int) (TreeNode, error) {
	node := TreeNode{Name: filepath.Base(path.absolute), Type: "directory"}
	entries, err := path.ReadDir()
	if err != nil {
		return TreeNode{}, err
	}
	if depth >= maxDepth {
		node.Truncated = true
		return node, nil
	}
	children := make([]TreeNode, 0)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
			continue
		}
		if entry.IsDir() {
			childRoot, err := path.OpenChild(entry.Name())
			if err != nil {
				continue
			}
			child, err := buildTree(childRoot, depth+1, maxDepth)
			_ = childRoot.Close()
			if err != nil {
				return TreeNode{}, err
			}
			children = append(children, child)
		} else {
			children = append(children, TreeNode{Name: entry.Name(), Type: "file"})
		}
	}
	node.Children = children
	return node, nil
}

func globRegexp(pattern string) (*regexp.Regexp, error) {
	normalized := filepath.ToSlash(pattern)
	var builder strings.Builder
	builder.WriteString("(?i)^")
	for i := 0; i < len(normalized); {
		if i+1 < len(normalized) && normalized[i:i+2] == "**" {
			builder.WriteString(".*")
			i += 2
			continue
		}
		switch normalized[i] {
		case '*':
			builder.WriteString(`[^/]*`)
		case '?':
			builder.WriteString(`[^/]`)
		default:
			builder.WriteString(regexp.QuoteMeta(string(normalized[i])))
		}
		i++
	}
	builder.WriteString("$")
	return regexp.Compile(builder.String())
}

func simpleGlobRegexp(pattern string) (*regexp.Regexp, error) {
	var builder strings.Builder
	builder.WriteString("(?i)^")
	for _, char := range pattern {
		switch char {
		case '*':
			builder.WriteString(".*")
		case '?':
			builder.WriteString(".")
		default:
			builder.WriteString(regexp.QuoteMeta(string(char)))
		}
	}
	builder.WriteString("$")
	return regexp.Compile(builder.String())
}

func ignoreMatcher(pattern string) (*regexp.Regexp, error) {
	return simpleGlobRegexp(pattern)
}

func skipGlobDirectory(name string) bool {
	switch name {
	case "node_modules", ".git", "dist", "build":
		return true
	default:
		return false
	}
}
