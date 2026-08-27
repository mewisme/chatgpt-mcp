package memory

import "os"

type Store struct{ Path string }

func (s Store) Read() (string, error) {
	data, err := os.ReadFile(s.Path)
	return string(data), err
}

func (s Store) Write(value string) error { return os.WriteFile(s.Path, []byte(value), 0600) }
