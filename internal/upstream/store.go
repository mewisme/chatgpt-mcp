package upstream

import (
	"os"
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

type Store struct{ Path string }

type diskStore struct {
	Servers []Server `json:"servers"`
}

func NewStore(path string) *Store { return &Store{Path: path} }

func (s *Store) Load() ([]Server, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return []Server{}, nil
	}
	if err != nil {
		return nil, err
	}
	var stored diskStore
	if err := configformat.UnmarshalPath(s.Path, data, &stored); err != nil {
		format, formatErr := configformat.Detect(s.Path)
		if formatErr != nil || format != configformat.JSON {
			return nil, err
		}
		var legacy []Server
		if legacyErr := configformat.UnmarshalPath(s.Path, data, &legacy); legacyErr != nil {
			return nil, err
		}
		return legacy, nil
	}
	if stored.Servers == nil {
		stored.Servers = []Server{}
	}
	return stored.Servers, nil
}

func (s *Store) Save(servers []Server) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	data, err := configformat.MarshalPath(s.Path, diskStore{Servers: servers})
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(s.Path, data, 0600)
}
