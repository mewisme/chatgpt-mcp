package oauth

import "sync"

type Store struct { mu sync.RWMutex; tokens map[string]string }

func NewStore() *Store { return &Store{tokens: map[string]string{}} }
func (s *Store) Set(id, token string) { s.mu.Lock(); defer s.mu.Unlock(); s.tokens[id]=token }
func (s *Store) Get(id string) (string,bool) { s.mu.RLock(); defer s.mu.RUnlock(); v,ok:=s.tokens[id]; return v,ok }
