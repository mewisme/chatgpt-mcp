package oauth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/secretstore"
	statepkg "go.mewis.me/chatgpt-mcp/internal/state"
)

const storeVersion = 1

type diskStore struct {
	Version     int                   `json:"version"`
	Credentials map[string]Credential `json:"credentials"`
}

func (s *Store) SecretEntries() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readDiskLocked()
	if err != nil {
		return nil, err
	}
	entries := []string{}
	for id, credential := range state.Credentials {
		if secretstore.IsMarker(credential.ClientSecret) {
			entries = append(entries, oauthSecretName(id, "client-secret"))
		}
		if secretstore.IsMarker(credential.AccessToken) {
			entries = append(entries, oauthSecretName(id, "access-token"))
		}
		if secretstore.IsMarker(credential.RefreshToken) {
			entries = append(entries, oauthSecretName(id, "refresh-token"))
		}
	}
	return entries, nil
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
	previous, err := s.readLocked()
	if err != nil {
		return err
	}
	next := cloneDiskStore(previous)
	value.UpdatedAt = time.Now().UTC()
	next.Credentials[value.ServerID] = cloneCredential(value)
	return s.writeLocked(previous, next)
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, err := s.readLocked()
	if err != nil {
		return err
	}
	if _, ok := previous.Credentials[id]; !ok {
		return nil
	}
	next := cloneDiskStore(previous)
	delete(next.Credentials, id)
	return s.writeLocked(previous, next)
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
	raw, err := s.readDiskLocked()
	if err != nil {
		return diskStore{}, err
	}
	state := cloneDiskStore(raw)
	migrate := false
	for id, credential := range state.Credentials {
		var migrated bool
		credential.ClientSecret, migrated, err = s.loadSecret(id, "client-secret", raw.Credentials[id].ClientSecret)
		if err != nil {
			return diskStore{}, err
		}
		migrate = migrate || migrated
		credential.AccessToken, migrated, err = s.loadSecret(id, "access-token", raw.Credentials[id].AccessToken)
		if err != nil {
			return diskStore{}, err
		}
		migrate = migrate || migrated
		credential.RefreshToken, migrated, err = s.loadSecret(id, "refresh-token", raw.Credentials[id].RefreshToken)
		if err != nil {
			return diskStore{}, err
		}
		migrate = migrate || migrated
		state.Credentials[id] = credential
	}
	if migrate {
		if err := s.writeLocked(raw, state); err != nil {
			return diskStore{}, fmt.Errorf("migrate OAuth secrets to secret file store: %w", err)
		}
	}
	return state, nil
}

func (s *Store) readDiskLocked() (diskStore, error) {
	state := diskStore{Version: storeVersion, Credentials: map[string]Credential{}}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return diskStore{}, fmt.Errorf("read oauth store: %w", err)
	}
	if err := configformat.UnmarshalPath(s.path, data, &state); err != nil {
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

func (s *Store) loadSecret(id, field, stored string) (string, bool, error) {
	if stored == "" {
		return "", false, nil
	}
	if !secretstore.IsMarker(stored) {
		return stored, true, nil
	}
	value, err := s.secrets.Get(oauthSecretName(id, field))
	if errors.Is(err, secretstore.ErrNotFound) {
		return "", false, fmt.Errorf("OAuth %s for %s is configured but missing from secret file store", field, id)
	}
	if err != nil {
		return "", false, err
	}
	return value, false, nil
}

func (s *Store) writeLocked(previous, next diskStore) error {
	persisted := cloneDiskStore(next)
	for id, credential := range persisted.Credentials {
		if credential.ClientSecret != "" {
			credential.ClientSecret = secretstore.Marker
		}
		if credential.AccessToken != "" {
			credential.AccessToken = secretstore.Marker
		}
		if credential.RefreshToken != "" {
			credential.RefreshToken = secretstore.Marker
		}
		persisted.Credentials[id] = credential
	}
	data, err := configformat.MarshalPath(s.path, persisted)
	if err != nil {
		return fmt.Errorf("encode oauth store: %w", err)
	}
	snapshot, err := snapshotOAuthFile(s.path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create oauth store directory: %w", err)
	}
	if err := statepkg.WriteFileAtomic(s.path, data, 0600); err != nil {
		return fmt.Errorf("write oauth store: %w", err)
	}
	if err := s.secrets.Apply(oauthSecretChanges(previous, next)); err != nil {
		return errors.Join(err, restoreOAuthFile(s.path, snapshot))
	}
	return nil
}

func oauthSecretChanges(previous, next diskStore) []secretstore.Change {
	ids := map[string]bool{}
	for id := range previous.Credentials {
		ids[id] = true
	}
	for id := range next.Credentials {
		ids[id] = true
	}
	changes := []secretstore.Change{}
	for id := range ids {
		old := previous.Credentials[id]
		value, exists := next.Credentials[id]
		for _, field := range []struct{ name, old, next string }{
			{"client-secret", old.ClientSecret, value.ClientSecret}, {"access-token", old.AccessToken, value.AccessToken}, {"refresh-token", old.RefreshToken, value.RefreshToken},
		} {
			if field.next != "" || field.old != "" || !exists {
				changes = append(changes, secretstore.Change{Name: oauthSecretName(id, field.name), Value: field.next})
			}
		}
	}
	return changes
}

func oauthSecretName(id, field string) string { return secretstore.Name("oauth", id, field) }

func cloneDiskStore(value diskStore) diskStore {
	result := diskStore{Version: value.Version, Credentials: make(map[string]Credential, len(value.Credentials))}
	for id, credential := range value.Credentials {
		result.Credentials[id] = cloneCredential(credential)
	}
	return result
}

func cloneCredential(value Credential) Credential {
	value.Scopes = append([]string(nil), value.Scopes...)
	return value
}

type oauthFileSnapshot struct {
	exists bool
	data   []byte
	mode   os.FileMode
}

func snapshotOAuthFile(path string) (oauthFileSnapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return oauthFileSnapshot{}, nil
	}
	if err != nil {
		return oauthFileSnapshot{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return oauthFileSnapshot{}, err
	}
	return oauthFileSnapshot{exists: true, data: data, mode: info.Mode().Perm()}, nil
}
func restoreOAuthFile(path string, snapshot oauthFileSnapshot) error {
	if !snapshot.exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return statepkg.WriteFileAtomic(path, snapshot.data, snapshot.mode)
}
