package oauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const storeVersion = 1

type diskStore struct {
	Version     int                   `json:"version"`
	Credentials map[string]Credential `json:"credentials"`
}

func (s *Store) Get(id string) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return Credential{}, err
	}
	value, ok := state.Credentials[id]
	if !ok {
		return Credential{}, ErrCredentialNotFound
	}
	return cloneCredential(value), nil
}

func (s *Store) Put(value Credential) error {
	if value.ServerID == "" {
		return errors.New("oauth server id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return err
	}
	value.UpdatedAt = time.Now().UTC()
	state.Credentials[value.ServerID] = cloneCredential(value)
	return s.writeLocked(state)
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return err
	}
	if _, ok := state.Credentials[id]; !ok {
		return nil
	}
	delete(state.Credentials, id)
	return s.writeLocked(state)
}

func (s *Store) Status(id string) (Status, error) {
	credential, err := s.Get(id)
	if errors.Is(err, ErrCredentialNotFound) {
		return Status{ServerID: id}, nil
	}
	if err != nil {
		return Status{}, err
	}
	status := Status{
		ServerID: id, Configured: true, Issuer: credential.Issuer, Resource: credential.Resource,
		Registration: credential.Registration, ClientID: credential.ClientID, Scopes: append([]string(nil), credential.Scopes...),
		HasRefreshToken: credential.RefreshToken != "",
	}
	if !credential.ExpiresAt.IsZero() {
		expiresAt := credential.ExpiresAt
		status.ExpiresAt = &expiresAt
		status.Expired = !time.Now().Before(expiresAt)
	}
	return status, nil
}

func (s *Store) readLocked() (diskStore, error) {
	state := diskStore{Version: storeVersion, Credentials: map[string]Credential{}}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return diskStore{}, fmt.Errorf("read oauth store: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return diskStore{}, fmt.Errorf("decode oauth store: %w", err)
	}
	if state.Version != storeVersion {
		return diskStore{}, fmt.Errorf("unsupported oauth store version: %d", state.Version)
	}
	if state.Credentials == nil {
		state.Credentials = map[string]Credential{}
	}
	return state, nil
}

func (s *Store) writeLocked(state diskStore) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create oauth store directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("chmod oauth store directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode oauth store: %w", err)
	}
	data = append(data, '\n')
	file, err := os.CreateTemp(dir, ".oauth-*.tmp")
	if err != nil {
		return fmt.Errorf("create oauth store temp file: %w", err)
	}
	temp := file.Name()
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(temp)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod oauth store temp file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write oauth store: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync oauth store: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close oauth store: %w", err)
	}
	if err := os.Rename(temp, s.path); err != nil {
		if runtime.GOOS != "windows" {
			return fmt.Errorf("replace oauth store: %w", err)
		}
		if removeErr := os.Remove(s.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("replace oauth store: %w", err)
		}
		if err := os.Rename(temp, s.path); err != nil {
			return fmt.Errorf("replace oauth store: %w", err)
		}
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("chmod oauth store: %w", err)
	}
	ok = true
	return nil
}

func cloneCredential(value Credential) Credential {
	value.Scopes = append([]string(nil), value.Scopes...)
	return value
}
