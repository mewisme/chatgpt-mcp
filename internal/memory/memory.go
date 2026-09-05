package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

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

type entry struct {
	Scope string
	Note  string
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

func (s Store) Upsert(workspaceID, scope, note string) (string, error) {
	scope = normalizeLine(strings.TrimLeft(strings.TrimSpace(scope), "#"))
	if scope == "" {
		return "", errors.New("scope is required")
	}
	note = normalizeLine(note)
	if note == "" {
		return "", errors.New("note is required")
	}
	path := s.WorkspacePath(workspaceID)
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	entries := parseEntries(string(existing))
	updated := false
	for index := range entries {
		if strings.EqualFold(entries[index].Scope, scope) {
			entries[index] = entry{Scope: scope, Note: note}
			updated = true
			break
		}
	}
	if !updated {
		entries = append(entries, entry{Scope: scope, Note: note})
	}
	if err := state.WriteFileAtomic(path, []byte(formatEntries(entries)), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func normalizeLine(value string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " ")), " ")
}

func parseEntries(value string) []entry {
	entries := []entry{}
	currentScope := ""
	legacy := []string{}
	for _, raw := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "## ") {
			currentScope = normalizeLine(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			continue
		}
		if line == "" || strings.HasPrefix(line, "# ") {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			note := normalizeLine(strings.TrimPrefix(line, "- "))
			if currentScope == "" {
				note = stripLegacyDate(note)
				if note != "" {
					legacy = append(legacy, note)
				}
				continue
			}
			entries = mergeParsedEntry(entries, currentScope, note)
			continue
		}
		if currentScope != "" {
			entries = mergeParsedEntry(entries, currentScope, normalizeLine(line))
		}
	}
	if len(legacy) > 0 {
		entries = append([]entry{{Scope: "general", Note: normalizeLine(strings.Join(legacy, " "))}}, entries...)
	}
	return entries
}

func mergeParsedEntry(entries []entry, scope, note string) []entry {
	if scope == "" || note == "" {
		return entries
	}
	for index := range entries {
		if strings.EqualFold(entries[index].Scope, scope) {
			entries[index].Note = normalizeLine(entries[index].Note + " " + note)
			return entries
		}
	}
	return append(entries, entry{Scope: scope, Note: note})
}

func stripLegacyDate(note string) string {
	if len(note) >= 12 && note[4] == '-' && note[7] == '-' && note[10] == ':' {
		return normalizeLine(note[11:])
	}
	return note
}

func formatEntries(entries []entry) string {
	var builder strings.Builder
	for _, item := range entries {
		if item.Scope == "" || item.Note == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("## ")
		builder.WriteString(item.Scope)
		builder.WriteString("\n- ")
		builder.WriteString(item.Note)
		builder.WriteByte('\n')
	}
	return builder.String()
}
