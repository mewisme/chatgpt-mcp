package tools

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxBinaryChunk = 8 * 1024 * 1024

type ReadTextFileResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Offset  *int   `json:"offset,omitempty"`
	Limit   *int   `json:"limit,omitempty"`
	Lines   *int   `json:"lines,omitempty"`
	Head    *int   `json:"head,omitempty"`
	Tail    *int   `json:"tail,omitempty"`
}

type ReadFileBase64Result struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	Offset     int64  `json:"offset"`
	BytesRead  int    `json:"bytes_read"`
	NextOffset *int64 `json:"next_offset"`
	Done       bool   `json:"done"`
	Encoding   string `json:"encoding"`
	Content    string `json:"content"`
}

type WriteFileResult struct {
	Path         string  `json:"path"`
	Bytes        int     `json:"bytes"`
	CheckpointID *string `json:"checkpoint_id"`
}

type EditFileResult struct {
	Path         string  `json:"path"`
	Diff         string  `json:"diff"`
	DryRun       bool    `json:"dry_run"`
	CheckpointID *string `json:"checkpoint_id"`
}

type MultiEditResult struct {
	Path         string  `json:"path"`
	Diff         string  `json:"diff"`
	Edits        int     `json:"edits"`
	DryRun       bool    `json:"dry_run"`
	CheckpointID *string `json:"checkpoint_id"`
}

type ApplyPatchResult struct {
	Path         string           `json:"path,omitempty"`
	Diff         string           `json:"diff,omitempty"`
	Files        []map[string]any `json:"files,omitempty"`
	DryRun       bool             `json:"dry_run"`
	MultiFile    bool             `json:"multi_file,omitempty"`
	CheckpointID *string          `json:"checkpoint_id"`
}

type DirectoryItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ListDirectoryResult struct {
	Path    string          `json:"path"`
	Entries []DirectoryItem `json:"entries"`
	Count   int             `json:"count"`
}

type GlobResult struct {
	Path    string   `json:"path"`
	Pattern string   `json:"pattern"`
	Matches []string `json:"matches"`
	Count   int      `json:"count"`
}

type GrepResult struct {
	Path       string `json:"path"`
	Pattern    string `json:"pattern"`
	OutputMode string `json:"output_mode"`
	Output     string `json:"output"`
}

type DeleteResult struct {
	Path         string  `json:"path"`
	CheckpointID *string `json:"checkpoint_id"`
}

type CreateDirectoryResult struct {
	Path string `json:"path"`
}

type CopyMoveResult struct {
	Source       string  `json:"source"`
	Destination  string  `json:"destination"`
	CheckpointID *string `json:"checkpoint_id"`
}

type SearchFilesResult struct {
	Path    string   `json:"path"`
	Pattern string   `json:"pattern"`
	Matches []string `json:"matches"`
	Count   int      `json:"count"`
}

type DirectoryTreeResult struct {
	Path     string   `json:"path"`
	Tree     TreeNode `json:"tree"`
	MaxDepth int      `json:"max_depth"`
}

type AllowedDirectoriesResult struct {
	FullMachineAccess bool     `json:"full_machine_access"`
	Permission        string   `json:"permission"`
	DefaultCWD        string   `json:"default_cwd"`
	MachineRoots      []string `json:"machine_roots"`
	WorkspaceID       string   `json:"workspace_id"`
	WorkspaceRoot     string   `json:"workspace_root"`
}

type EditSpec struct {
	OldText    string
	NewText    string
	ReplaceAll bool
}

func checkpointPointer(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

func readTextSlice(content string, offset, limit, head, tail *int) ReadTextFileResult {
	lines := strings.Split(content, "\n")
	result := ReadTextFileResult{}
	if offset != nil {
		start := *offset - 1
		if start < 0 {
			start = 0
		}
		if start > len(lines) {
			start = len(lines)
		}
		end := len(lines)
		if limit != nil && start+*limit < end {
			end = start + *limit
		}
		slice := lines[start:end]
		numbered := make([]string, len(slice))
		for i, line := range slice {
			numbered[i] = fmt.Sprintf("%6d|%s", start+i+1, line)
		}
		count := len(slice)
		result.Content = strings.Join(numbered, "\n")
		result.Offset = offset
		result.Limit = limit
		result.Lines = &count
		return result
	}
	switch {
	case head != nil:
		count := *head
		if count < 0 {
			count = 0
		}
		if count > len(lines) {
			count = len(lines)
		}
		result.Content = strings.Join(lines[:count], "\n")
		result.Head = head
	case tail != nil:
		count := *tail
		if count < 0 {
			count = 0
		}
		if count > len(lines) {
			count = len(lines)
		}
		result.Content = strings.Join(lines[len(lines)-count:], "\n")
		result.Tail = tail
	default:
		result.Content = content
	}
	return result
}

func readBase64Chunk(path string, offset int64, length int) (ReadFileBase64Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ReadFileBase64Result{}, err
	}
	if !info.Mode().IsRegular() {
		return ReadFileBase64Result{}, errors.New("path is not a regular file")
	}
	if offset < 0 {
		return ReadFileBase64Result{}, errors.New("offset must be non-negative")
	}
	if offset > info.Size() {
		offset = info.Size()
	}
	if length <= 0 {
		return ReadFileBase64Result{}, errors.New("length must be positive")
	}
	if length > maxBinaryChunk {
		length = maxBinaryChunk
	}
	remaining := info.Size() - offset
	if int64(length) > remaining {
		length = int(remaining)
	}
	file, err := os.Open(path)
	if err != nil {
		return ReadFileBase64Result{}, err
	}
	defer file.Close()
	buffer := make([]byte, length)
	bytesRead, err := file.ReadAt(buffer, offset)
	if err != nil && !errors.Is(err, os.ErrClosed) && bytesRead == 0 && length > 0 {
		return ReadFileBase64Result{}, err
	}
	next := offset + int64(bytesRead)
	var nextOffset *int64
	if next < info.Size() {
		value := next
		nextOffset = &value
	}
	return ReadFileBase64Result{
		Path: path, Size: info.Size(), Offset: offset, BytesRead: bytesRead, NextOffset: nextOffset,
		Done: next >= info.Size(), Encoding: "base64", Content: base64.StdEncoding.EncodeToString(buffer[:bytesRead]),
	}, nil
}

func decodeBase64(value string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 content: %w", err)
	}
	return data, nil
}

func replaceExact(content, oldText, newText string, replaceAll bool) (string, error) {
	if !strings.Contains(content, oldText) {
		return "", errors.New("old_text not found in file. Ensure exact match")
	}
	if replaceAll {
		return strings.ReplaceAll(content, oldText, newText), nil
	}
	return strings.Replace(content, oldText, newText, 1), nil
}

func replaceRegex(content, pattern, replacement, flags string) (string, error) {
	global := strings.Contains(flags, "g")
	inline := ""
	for _, flag := range flags {
		switch flag {
		case 'g', 'u':
		case 'i', 'm', 's':
			if !strings.ContainsRune(inline, flag) {
				inline += string(flag)
			}
		default:
			return "", fmt.Errorf("unsupported regex flag: %c", flag)
		}
	}
	if inline != "" {
		pattern = "(?" + inline + ")" + pattern
	}
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	template := strings.ReplaceAll(replacement, "$&", "${0}")
	template = regexp.MustCompile(`\$<([A-Za-z_][A-Za-z0-9_]*)>`).ReplaceAllString(template, "${$1}")
	if global {
		next := regex.ReplaceAllString(content, template)
		if next == content {
			return "", errors.New("regex made no changes")
		}
		return next, nil
	}
	index := regex.FindStringSubmatchIndex(content)
	if index == nil {
		return "", errors.New("regex made no changes")
	}
	var expanded []byte
	expanded = regex.ExpandString(expanded, template, content, index)
	next := content[:index[0]] + string(expanded) + content[index[1]:]
	if next == content {
		return "", errors.New("regex made no changes")
	}
	return next, nil
}

func pathType(entry os.DirEntry) string {
	if entry.IsDir() {
		return "directory"
	}
	return "file"
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
