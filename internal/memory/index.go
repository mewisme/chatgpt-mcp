package memory

import (
	"sort"
	"strings"
	"sync"
)

type Query struct {
	Text  string
	Scope string
	Limit int
}

type Match struct {
	Entry Entry   `json:"entry"`
	Score float64 `json:"score"`
}

type Index interface {
	Rebuild(workspaceID string, entries []Entry) error
	Upsert(workspaceID string, entry Entry) error
	Delete(workspaceID, scope, key string) error
	Search(workspaceID string, query Query) ([]Match, error)
}

type MemoryIndex struct {
	mu         sync.RWMutex
	workspaces map[string]map[string]Entry
}

func NewMemoryIndex() *MemoryIndex {
	return &MemoryIndex{workspaces: map[string]map[string]Entry{}}
}

func (i *MemoryIndex) Rebuild(workspaceID string, entries []Entry) error {
	workspaceID = strings.TrimSpace(workspaceID)
	values := map[string]Entry{}
	for _, item := range normalizeDocument(Document{Entries: entries}).Entries {
		values[indexKey(item.Scope, item.Key)] = item
	}
	i.mu.Lock()
	i.workspaces[workspaceID] = values
	i.mu.Unlock()
	return nil
}

func (i *MemoryIndex) Upsert(workspaceID string, entry Entry) error {
	entry = normalizeDocument(Document{Entries: []Entry{entry}}).first()
	if entry.Scope == "" {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	values := i.workspaces[workspaceID]
	if values == nil {
		values = map[string]Entry{}
		i.workspaces[workspaceID] = values
	}
	values[indexKey(entry.Scope, entry.Key)] = entry
	return nil
}

func (i *MemoryIndex) Delete(workspaceID, scope, key string) error {
	scope = normalizeName(scope)
	hasKey := strings.TrimSpace(key) != ""
	key = canonicalKey(scope, key)
	i.mu.Lock()
	defer i.mu.Unlock()
	values := i.workspaces[workspaceID]
	if hasKey {
		delete(values, indexKey(scope, key))
		return nil
	}
	for id, item := range values {
		if strings.EqualFold(item.Scope, scope) {
			delete(values, id)
		}
	}
	return nil
}

func (i *MemoryIndex) Search(workspaceID string, query Query) ([]Match, error) {
	i.mu.RLock()
	values := i.workspaces[workspaceID]
	entries := make([]Entry, 0, len(values))
	for _, item := range values {
		entries = append(entries, item)
	}
	i.mu.RUnlock()
	scope := normalizeName(query.Scope)
	matches := make([]Match, 0, len(entries))
	for _, item := range entries {
		if scope != "" && !strings.EqualFold(item.Scope, scope) {
			continue
		}
		score := lexicalScore(item, query.Text)
		if strings.TrimSpace(query.Text) != "" && score <= 0 {
			continue
		}
		matches = append(matches, Match{Entry: item, Score: score})
	}
	sort.SliceStable(matches, func(a, b int) bool {
		if matches[a].Score != matches[b].Score {
			return matches[a].Score > matches[b].Score
		}
		left, right := indexKey(matches[a].Entry.Scope, matches[a].Entry.Key), indexKey(matches[b].Entry.Scope, matches[b].Entry.Key)
		return left < right
	})
	if query.Limit > 0 && len(matches) > query.Limit {
		matches = matches[:query.Limit]
	}
	return matches, nil
}

func (d Document) first() Entry {
	if len(d.Entries) == 0 {
		return Entry{}
	}
	return d.Entries[0]
}

func indexKey(scope, key string) string {
	return strings.ToLower(normalizeName(scope)) + "\x00" + strings.ToLower(normalizeName(key))
}

func lexicalScore(entry Entry, query string) float64 {
	queryTokens := tokenSet(query)
	if len(queryTokens) == 0 {
		return 0
	}
	noteTokens, keyTokens, scopeTokens := tokenSet(entry.Note), tokenSet(entry.Key), tokenSet(entry.Scope)
	score := 0.0
	for token := range queryTokens {
		if _, ok := noteTokens[token]; ok {
			score += 1
		}
		if _, ok := keyTokens[token]; ok {
			score += 2
		}
		if _, ok := scopeTokens[token]; ok {
			score += 1.5
		}
	}
	return score / float64(len(queryTokens))
}

func tokenSet(value string) map[string]struct{} {
	tokens := map[string]struct{}{}
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r < 128
	}) {
		if token != "" {
			tokens[token] = struct{}{}
		}
	}
	return tokens
}
