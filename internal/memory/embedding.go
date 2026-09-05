package memory

import (
	"context"
	"errors"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"sync"
)

const DefaultEmbeddingDimensions = 256

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type NGramEmbedder struct{ Dimensions int }

func NewLocalEmbedder() NGramEmbedder { return NGramEmbedder{Dimensions: DefaultEmbeddingDimensions} }

func (e NGramEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	dimensions := e.Dimensions
	if dimensions <= 0 {
		dimensions = DefaultEmbeddingDimensions
	}
	vectors := make([][]float32, len(texts))
	for index, text := range texts {
		vector := make([]float32, dimensions)
		features := embeddingFeatures(text)
		for _, feature := range features {
			hash := fnv.New64a()
			_, _ = hash.Write([]byte(feature))
			value := hash.Sum64()
			bucket := int(value % uint64(dimensions))
			sign := float32(1)
			if value&(1<<63) != 0 {
				sign = -1
			}
			vector[bucket] += sign
		}
		normalizeVector(vector)
		vectors[index] = vector
	}
	return vectors, nil
}

type HybridWeights struct {
	Embedding float64
	Lexical   float64
	Boost     float64
}

func DefaultHybridWeights() HybridWeights {
	return HybridWeights{Embedding: 0.65, Lexical: 0.25, Boost: 0.10}
}

type HybridIndex struct {
	base     *MemoryIndex
	embedder Embedder
	weights  HybridWeights
	mu       sync.RWMutex
	vectors  map[string]map[string][]float32
}

func NewHybridIndex(embedder Embedder, weights HybridWeights) *HybridIndex {
	if embedder == nil {
		embedder = NewLocalEmbedder()
	}
	if weights == (HybridWeights{}) {
		weights = DefaultHybridWeights()
	}
	return &HybridIndex{base: NewMemoryIndex(), embedder: embedder, weights: weights, vectors: map[string]map[string][]float32{}}
}

func (i *HybridIndex) Rebuild(workspaceID string, entries []Entry) error {
	entries = normalizeDocument(Document{Entries: entries}).Entries
	if err := i.base.Rebuild(workspaceID, entries); err != nil {
		return err
	}
	texts := make([]string, len(entries))
	for index, entry := range entries {
		texts[index] = embeddingText(entry)
	}
	vectors, err := i.embedder.Embed(context.Background(), texts)
	if err != nil {
		return err
	}
	if len(vectors) != len(entries) {
		return errors.New("embedder returned unexpected vector count")
	}
	values := map[string][]float32{}
	for index, entry := range entries {
		values[indexKey(entry.Scope, entry.Key)] = vectors[index]
	}
	i.mu.Lock()
	i.vectors[workspaceID] = values
	i.mu.Unlock()
	return nil
}

func (i *HybridIndex) Upsert(workspaceID string, entry Entry) error {
	if err := i.base.Upsert(workspaceID, entry); err != nil {
		return err
	}
	vectors, err := i.embedder.Embed(context.Background(), []string{embeddingText(entry)})
	if err != nil {
		return err
	}
	if len(vectors) != 1 {
		return errors.New("embedder returned unexpected vector count")
	}
	i.mu.Lock()
	values := i.vectors[workspaceID]
	if values == nil {
		values = map[string][]float32{}
		i.vectors[workspaceID] = values
	}
	values[indexKey(entry.Scope, entry.Key)] = vectors[0]
	i.mu.Unlock()
	return nil
}

func (i *HybridIndex) Delete(workspaceID, scope, key string) error {
	hasKey := strings.TrimSpace(key) != ""
	canonical := canonicalKey(scope, key)
	if err := i.base.Delete(workspaceID, scope, key); err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	values := i.vectors[workspaceID]
	if hasKey {
		delete(values, indexKey(scope, canonical))
		return nil
	}
	prefix := strings.ToLower(normalizeName(scope)) + "\x00"
	for id := range values {
		if strings.HasPrefix(id, prefix) {
			delete(values, id)
		}
	}
	return nil
}

func (i *HybridIndex) Search(workspaceID string, query Query) ([]Match, error) {
	queryVector, err := i.embedder.Embed(context.Background(), []string{strings.TrimSpace(query.Text)})
	if err != nil {
		return nil, err
	}
	all, err := i.base.Search(workspaceID, Query{Scope: query.Scope})
	if err != nil {
		return nil, err
	}
	i.mu.RLock()
	vectors := i.vectors[workspaceID]
	i.mu.RUnlock()
	matches := make([]Match, 0, len(all))
	for _, candidate := range all {
		entry := candidate.Entry
		lexical := lexicalScore(entry, query.Text)
		embedding := 0.0
		if len(queryVector) == 1 {
			embedding = cosine(queryVector[0], vectors[indexKey(entry.Scope, entry.Key)])
		}
		boost := exactBoost(entry, query.Text)
		score := embedding*i.weights.Embedding + math.Min(lexical/3.5, 1)*i.weights.Lexical + boost*i.weights.Boost
		if strings.TrimSpace(query.Text) != "" && score <= 0 {
			continue
		}
		matches = append(matches, Match{Entry: entry, Score: score})
	}
	sort.SliceStable(matches, func(a, b int) bool {
		if matches[a].Score != matches[b].Score {
			return matches[a].Score > matches[b].Score
		}
		return indexKey(matches[a].Entry.Scope, matches[a].Entry.Key) < indexKey(matches[b].Entry.Scope, matches[b].Entry.Key)
	})
	if query.Limit > 0 && len(matches) > query.Limit {
		matches = matches[:query.Limit]
	}
	return matches, nil
}

func embeddingText(entry Entry) string { return entry.Scope + " " + entry.Key + " " + entry.Note }

func embeddingFeatures(value string) []string {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	features := make([]string, 0, len(value))
	for _, token := range strings.Fields(value) {
		features = append(features, "w:"+token)
	}
	runes := []rune(value)
	for index := 0; index+2 < len(runes); index++ {
		features = append(features, "g:"+string(runes[index:index+3]))
	}
	return features
}

func normalizeVector(vector []float32) {
	sum := 0.0
	for _, value := range vector {
		sum += float64(value * value)
	}
	if sum == 0 {
		return
	}
	norm := float32(math.Sqrt(sum))
	for index := range vector {
		vector[index] /= norm
	}
}

func cosine(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	sum := float64(0)
	for index := range left {
		sum += float64(left[index] * right[index])
	}
	if sum < 0 {
		return 0
	}
	return math.Min(sum, 1)
}

func exactBoost(entry Entry, query string) float64 {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return 0
	}
	if entry.Key != "" && strings.Contains(query, strings.ToLower(entry.Key)) {
		return 1
	}
	if strings.Contains(query, strings.ToLower(entry.Scope)) {
		return 0.7
	}
	return 0
}
