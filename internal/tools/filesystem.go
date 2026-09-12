package tools

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxBinaryChunk        = 8 * 1024 * 1024
	maxTextReadBytes      = 4 * 1024 * 1024
	maxMutationFileBytes  = 16 * 1024 * 1024
	maxSearchFileBytes    = 16 * 1024 * 1024
	maxTextSelectionLines = 100_000
)

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

func readTextFile(path string, offset, limit, head, tail *int) (ReadTextFileResult, error) {
	if offset == nil && head == nil && tail == nil {
		data, err := readRegularFileLimited(path, maxTextReadBytes, "text read")
		if err != nil {
			return ReadTextFileResult{}, err
		}
		return readTextSlice(string(data), nil, nil, nil, nil), nil
	}
	file, err := os.Open(path) // #nosec G304 -- path is resolved and workspace-contained before this helper is called.
	if err != nil {
		return ReadTextFileResult{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ReadTextFileResult{}, err
	}
	if !info.Mode().IsRegular() {
		return ReadTextFileResult{}, errors.New("path is not a regular file")
	}
	reader := bufio.NewReader(file)
	if offset != nil {
		return readTextOffset(reader, offset, limit)
	}
	if head != nil {
		return readTextHead(reader, head)
	}
	return readTextTail(reader, tail)
}

func readTextOffset(reader *bufio.Reader, offset, limit *int) (ReadTextFileResult, error) {
	start := *offset
	if start < 1 {
		start = 1
	}
	want := maxTextSelectionLines
	if limit != nil {
		want = *limit
	}
	lines := make([]string, 0, min(want, 256))
	lineNumber := 1
	bytes := 0
	for len(lines) < want {
		capture := lineNumber >= start
		line, ok, err := readLogicalLine(reader, capture, maxTextReadBytes-bytes)
		if err != nil {
			return ReadTextFileResult{}, err
		}
		if !ok {
			break
		}
		if capture {
			numbered := fmt.Sprintf("%6d|%s", lineNumber, line)
			if len(lines) > 0 {
				bytes++
			}
			bytes += len(numbered)
			if bytes > maxTextReadBytes {
				return ReadTextFileResult{}, textOutputLimitError()
			}
			lines = append(lines, numbered)
		}
		lineNumber++
	}
	if limit == nil && len(lines) == maxTextSelectionLines {
		if _, ok, err := readLogicalLine(reader, false, 0); err != nil {
			return ReadTextFileResult{}, err
		} else if ok {
			return ReadTextFileResult{}, fmt.Errorf("text read exceeds %d-line selection limit; provide limit to narrow the result", maxTextSelectionLines)
		}
	}
	count := len(lines)
	return ReadTextFileResult{Content: strings.Join(lines, "\n"), Offset: offset, Limit: limit, Lines: &count}, nil
}

func readTextHead(reader *bufio.Reader, head *int) (ReadTextFileResult, error) {
	want := *head
	lines := make([]string, 0, min(want, 256))
	bytes := 0
	for len(lines) < want {
		line, ok, err := readLogicalLine(reader, true, maxTextReadBytes-bytes)
		if err != nil {
			return ReadTextFileResult{}, err
		}
		if !ok {
			break
		}
		if len(lines) > 0 {
			bytes++
		}
		bytes += len(line)
		if bytes > maxTextReadBytes {
			return ReadTextFileResult{}, textOutputLimitError()
		}
		lines = append(lines, line)
	}
	return ReadTextFileResult{Content: strings.Join(lines, "\n"), Head: head}, nil
}

func readTextTail(reader *bufio.Reader, tail *int) (ReadTextFileResult, error) {
	want := *tail
	if want == 0 {
		return ReadTextFileResult{Content: "", Tail: tail}, nil
	}
	ring := make([]string, 0, min(want, 256))
	bytes := 0
	truncatedByBytes := false
	for {
		line, ok, err := readLogicalLine(reader, true, maxTextReadBytes)
		if err != nil {
			return ReadTextFileResult{}, err
		}
		if !ok {
			break
		}
		lineBytes := len(line)
		if len(ring) > 0 {
			lineBytes++
		}
		ring = append(ring, line)
		bytes += lineBytes
		if len(ring) > want {
			bytes -= len(ring[0])
			if len(ring) > 1 {
				bytes--
			}
			ring = ring[1:]
		}
		for bytes > maxTextReadBytes && len(ring) > 0 {
			truncatedByBytes = true
			bytes -= len(ring[0])
			if len(ring) > 1 {
				bytes--
			}
			ring = ring[1:]
		}
	}
	if truncatedByBytes && len(ring) < want {
		return ReadTextFileResult{}, textOutputLimitError()
	}
	return ReadTextFileResult{Content: strings.Join(ring, "\n"), Tail: tail}, nil
}

func readLogicalLine(reader *bufio.Reader, capture bool, remaining int) (string, bool, error) {
	if remaining < 0 {
		return "", false, textOutputLimitError()
	}
	var builder strings.Builder
	for {
		fragment, err := reader.ReadSlice('\n')
		if capture {
			if builder.Len()+len(fragment) > remaining+1 {
				return "", false, textOutputLimitError()
			}
			builder.Write(fragment)
		}
		switch {
		case err == nil:
			line := builder.String()
			return strings.TrimSuffix(line, "\n"), true, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if len(fragment) == 0 && builder.Len() == 0 {
				return "", false, nil
			}
			return builder.String(), true, nil
		default:
			return "", false, err
		}
	}
}

func readRegularFileLimited(path string, maxBytes int64, operation string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("path is not a regular file")
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("%s exceeds %s limit (%d bytes); use a partial/chunked operation", operation, byteLimitLabel(maxBytes), info.Size())
	}
	return os.ReadFile(path) // #nosec G304 -- callers pass a workspace-resolved path and size is bounded above.
}

func validateMutationPayload(size int) error {
	if int64(size) > maxMutationFileBytes {
		return fmt.Errorf("mutation payload exceeds %s limit", byteLimitLabel(maxMutationFileBytes))
	}
	return nil
}

func textOutputLimitError() error {
	return fmt.Errorf("text read exceeds %s output limit; reduce offset/limit/head/tail or use read_file_base64", byteLimitLabel(maxTextReadBytes))
}

func byteLimitLabel(bytes int64) string {
	if bytes%(1024*1024) == 0 {
		return fmt.Sprintf("%d MiB", bytes/(1024*1024))
	}
	return fmt.Sprintf("%d bytes", bytes)
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
	if int64(base64.StdEncoding.DecodedLen(len(value))) > maxMutationFileBytes {
		return nil, fmt.Errorf("decoded mutation payload exceeds %s limit", byteLimitLabel(maxMutationFileBytes))
	}
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

func copyFileContents(source, destination string) error {
	if filepath.Clean(source) == filepath.Clean(destination) {
		return nil
	}
	input, err := os.Open(source) // #nosec G304 -- source is workspace-resolved by the caller.
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("source is not a file")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil { // #nosec G301 -- workspace files intentionally preserve normal user-readable directory semantics.
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644) // #nosec G302,G304 -- workspace copy preserves existing 0644 file semantics; destination is workspace-resolved.
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
