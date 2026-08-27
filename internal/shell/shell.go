package shell

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type State struct { WorkingDirectory string `json:"working_directory"`; CWD string `json:"cwd"` }

type Store struct { Root string }

func (s Store) Path(id string) string { return filepath.Join(s.Root, "workspaces", id, "shell.json") }
func (s Store) Load(id string) (State, error) { var state State; data, err := os.ReadFile(s.Path(id)); if err != nil { return state, err }; err = json.Unmarshal(data, &state); return state, err }
func (s Store) Save(id string, state State) error { path := s.Path(id); if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil { return err }; data, _ := json.MarshalIndent(state, "", "  "); return os.WriteFile(path, data, 0600) }
