package upstream

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/secretstore"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

type Store struct {
	Path    string
	secrets *secretstore.Store
}

type diskStore struct {
	Servers []Server `json:"servers"`
}

func NewStore(path string) *Store {
	return &Store{Path: path, secrets: secretstore.New(filepath.Dir(path))}
}

func (s *Store) KeyringEntries() ([]string, error) {
	servers, err := s.readDisk()
	if err != nil {
		return nil, err
	}
	entries := []string{}
	for _, server := range servers {
		for key, value := range server.Headers {
			if SensitiveConfigKey(key) && value == secretstore.Marker {
				entries = append(entries, upstreamSecretName(server.ID, "header", key))
			}
		}
		for key, value := range server.Env {
			if SensitiveConfigKey(key) && value == secretstore.Marker {
				entries = append(entries, upstreamSecretName(server.ID, "env", key))
			}
		}
	}
	return entries, nil
}

func (s *Store) Load() ([]Server, error) {
	raw, err := s.readDisk()
	if err != nil {
		return nil, err
	}
	servers := cloneServers(raw)
	migrate := false
	for index := range servers {
		server := &servers[index]
		for key, stored := range server.Headers {
			if !SensitiveConfigKey(key) || stored == "" {
				continue
			}
			if stored != secretstore.Marker {
				migrate = true
				continue
			}
			value, err := s.secrets.Get(upstreamSecretName(server.ID, "header", key))
			if errors.Is(err, secretstore.ErrNotFound) {
				return nil, fmt.Errorf("upstream header %s for %s is configured but missing from OS keyring", key, server.ID)
			}
			if err != nil {
				return nil, err
			}
			server.Headers[key] = value
		}
		for key, stored := range server.Env {
			if !SensitiveConfigKey(key) || stored == "" {
				continue
			}
			if stored != secretstore.Marker {
				migrate = true
				continue
			}
			value, err := s.secrets.Get(upstreamSecretName(server.ID, "env", key))
			if errors.Is(err, secretstore.ErrNotFound) {
				return nil, fmt.Errorf("upstream env %s for %s is configured but missing from OS keyring", key, server.ID)
			}
			if err != nil {
				return nil, err
			}
			server.Env[key] = value
		}
	}
	if migrate {
		if err := s.saveWithPrevious(raw, servers); err != nil {
			return nil, fmt.Errorf("migrate upstream secrets to OS keyring: %w", err)
		}
	}
	return servers, nil
}

func (s *Store) Save(servers []Server) error {
	previous, err := s.readDisk()
	if err != nil {
		return err
	}
	return s.saveWithPrevious(previous, servers)
}

func (s *Store) readDisk() ([]Server, error) {
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

func (s *Store) saveWithPrevious(previous, servers []Server) error {
	persisted := cloneServers(servers)
	for index := range persisted {
		for key, value := range persisted[index].Headers {
			if SensitiveConfigKey(key) && value != "" {
				persisted[index].Headers[key] = secretstore.Marker
			}
		}
		for key, value := range persisted[index].Env {
			if SensitiveConfigKey(key) && value != "" {
				persisted[index].Env[key] = secretstore.Marker
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	data, err := configformat.MarshalPath(s.Path, diskStore{Servers: persisted})
	if err != nil {
		return err
	}
	snapshot, err := snapshotStoreFile(s.Path)
	if err != nil {
		return err
	}
	if err := state.WriteFileAtomic(s.Path, data, 0600); err != nil {
		return err
	}
	if err := s.secrets.Apply(upstreamSecretChanges(previous, servers)); err != nil {
		return errors.Join(err, restoreStoreFile(s.Path, snapshot))
	}
	return nil
}

func upstreamSecretChanges(previous, next []Server) []secretstore.Change {
	old := serversByID(previous)
	current := serversByID(next)
	ids := map[string]bool{}
	for id := range old {
		ids[id] = true
	}
	for id := range current {
		ids[id] = true
	}
	changes := []secretstore.Change{}
	for id := range ids {
		changes = append(changes, mapSecretChanges(id, "header", old[id].Headers, current[id].Headers)...)
		changes = append(changes, mapSecretChanges(id, "env", old[id].Env, current[id].Env)...)
	}
	return changes
}

func mapSecretChanges(id, kind string, previous, next map[string]string) []secretstore.Change {
	keys := map[string]bool{}
	for key := range previous {
		if SensitiveConfigKey(key) {
			keys[key] = true
		}
	}
	for key := range next {
		if SensitiveConfigKey(key) {
			keys[key] = true
		}
	}
	changes := make([]secretstore.Change, 0, len(keys))
	for key := range keys {
		changes = append(changes, secretstore.Change{Name: upstreamSecretName(id, kind, key), Value: next[key]})
	}
	return changes
}

func upstreamSecretName(id, kind, key string) string {
	return secretstore.Name("upstream", id, kind, key)
}

func serversByID(values []Server) map[string]Server {
	result := make(map[string]Server, len(values))
	for _, server := range values {
		result[server.ID] = server
	}
	return result
}

func cloneServers(values []Server) []Server {
	result := make([]Server, len(values))
	for index, server := range values {
		server.Args = append([]string(nil), server.Args...)
		server.Tools = append([]string(nil), server.Tools...)
		server.DisabledTools = append([]string(nil), server.DisabledTools...)
		server.Headers = cloneStringMap(server.Headers)
		server.Env = cloneStringMap(server.Env)
		result[index] = server
	}
	return result
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

type storeFileSnapshot struct {
	exists bool
	data   []byte
	mode   os.FileMode
}

func snapshotStoreFile(path string) (storeFileSnapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return storeFileSnapshot{}, nil
	}
	if err != nil {
		return storeFileSnapshot{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return storeFileSnapshot{}, err
	}
	return storeFileSnapshot{exists: true, data: data, mode: info.Mode().Perm()}, nil
}
func restoreStoreFile(path string, snapshot storeFileSnapshot) error {
	if !snapshot.exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return state.WriteFileAtomic(path, snapshot.data, snapshot.mode)
}
