package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

const (
	maxBytes = 25_000
	maxLines = 200
)

type Store struct {
	Path string
	Root string
}

func DefaultRoot() string {
	return configformat.RootPath()
}

func NewStore(root string) Store {
	return Store{Root: root}
}

func (s Store) Read() (string, error) {
	path := s.Path
	if path == "" {
		return "", errors.New("memory path is not configured")
	}
	data, err := os.ReadFile(path)
	return string(data), err
}

func (s Store) Write(value string) error {
	path := s.Path
	if path == "" {
		return errors.New("memory path is not configured")
	}
	return state.WriteFileAtomic(path, []byte(value), 0600)
}

func (s Store) WorkspacePath(workspaceID string) string {
	root := s.Root
	if root == "" {
		root = DefaultRoot()
	}
	return filepath.Join(root, "workspaces", workspaceID, "MEMORY.md")
}

func (s Store) Load(workspaceID string) (string, error) {
	data, err := os.ReadFile(s.WorkspacePath(workspaceID))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	value := strings.Join(lines, "\n")
	if len([]byte(value)) > maxBytes {
		value = string([]byte(value)[:maxBytes])
	}
	return strings.TrimSpace(value), nil
}

func (s Store) Append(workspaceID, note string) (string, error) {
	note = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(note, "\r", " "), "\n", " "))
	if note == "" {
		return "", errors.New("note is required")
	}
	path := s.WorkspacePath(workspaceID)
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	var builder strings.Builder
	if len(existing) == 0 {
		builder.WriteString("# Auto memory (cross-session notes)\n\n")
	} else {
		builder.Write(existing)
		if existing[len(existing)-1] != '\n' {
			builder.WriteByte('\n')
		}
	}
	builder.WriteString("- ")
	builder.WriteString(time.Now().UTC().Format("2006-01-02"))
	builder.WriteString(": ")
	builder.WriteString(note)
	builder.WriteByte('\n')
	if err := state.WriteFileAtomic(path, []byte(builder.String()), 0600); err != nil {
		return "", err
	}
	return path, nil
}
