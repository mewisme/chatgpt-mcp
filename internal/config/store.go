package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Store struct{ Path string }

func NewStore(path string) *Store { return &Store{Path: path} }

func (s *Store) Load() (map[string]any, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (s *Store) Save(value map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0600)
}
