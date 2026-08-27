package memory

import (
	"os"

	"go.mewis.me/chatgpt-mcp/internal/state"
)

type Store struct{ Path string }

func (s Store) Read() (string, error) {
	data, err := os.ReadFile(s.Path)
	return string(data), err
}

func (s Store) Write(value string) error { return state.WriteFileAtomic(s.Path, []byte(value), 0600) }
