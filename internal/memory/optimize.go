package memory

import (
	"os"
	"strings"
)

const (
	OversizedNoteBytes    = 400
	FragmentedNoteBytes   = 60
	OverlapThreshold      = 0.65
	ScopeEntriesSoftLimit = 12
	MemorySoftLimitBytes  = 16 * 1024
)

type OptimizationGroup struct {
	Scope   string  `json:"scope"`
	Entries []Entry `json:"entries"`
	Reason  string  `json:"reason"`
}

type OptimizationAnalysis struct {
	Groups                  []OptimizationGroup `json:"groups"`
	BeforeBytes             int                 `json:"before_bytes"`
	CandidateSavingsBytes   int                 `json:"candidate_savings_bytes"`
	LegacyFormat            bool                `json:"legacy_format"`
	OptimizationRecommended bool                `json:"optimization_recommended"`
}

func (s Store) Analyze(workspaceID, scope string) (OptimizationAnalysis, error) {
	path := s.WorkspacePath(workspaceID)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return OptimizationAnalysis{Groups: []OptimizationGroup{}}, nil
	}
	if err != nil {
		return OptimizationAnalysis{}, err
	}
	document := Parse(string(raw))
	scope = normalizeName(scope)
	entries := make([]Entry, 0, len(document.Entries))
	for _, entry := range document.Entries {
		if scope == "" || strings.EqualFold(entry.Scope, scope) {
			entries = append(entries, entry)
		}
	}
	analysis := AnalyzeEntries(entries)
	analysis.BeforeBytes = len(raw)
	analysis.LegacyFormat = strings.TrimSpace(string(raw)) != strings.TrimSpace(Render(document))
	if analysis.LegacyFormat {
		analysis.Groups = append(analysis.Groups, OptimizationGroup{Scope: scope, Entries: entries, Reason: "legacy format requires canonical migration"})
	}
	analysis.OptimizationRecommended = len(analysis.Groups) > 0
	return analysis, nil
}

func AnalyzeEntries(entries []Entry) OptimizationAnalysis {
	analysis := OptimizationAnalysis{Groups: []OptimizationGroup{}}
	byScope := map[string][]Entry{}
	for _, entry := range normalizeDocument(Document{Entries: entries}).Entries {
		id := strings.ToLower(entry.Scope)
		byScope[id] = append(byScope[id], entry)
		if len([]byte(entry.Note)) > OversizedNoteBytes {
			analysis.Groups = append(analysis.Groups, OptimizationGroup{Scope: entry.Scope, Entries: []Entry{entry}, Reason: "oversized note"})
			analysis.CandidateSavingsBytes += len([]byte(entry.Note)) - OversizedNoteBytes
		}
	}
	for _, scoped := range byScope {
		if len(scoped) >= 3 {
			fragments := make([]Entry, 0, len(scoped))
			for _, entry := range scoped {
				if len([]byte(entry.Note)) < FragmentedNoteBytes {
					fragments = append(fragments, entry)
				}
			}
			if len(fragments) >= 2 {
				analysis.Groups = append(analysis.Groups, OptimizationGroup{Scope: scoped[0].Scope, Entries: fragments, Reason: "fragmented small entries"})
			}
		}
		for left := 0; left < len(scoped); left++ {
			for right := left + 1; right < len(scoped); right++ {
				overlap := tokenOverlap(scoped[left].Note, scoped[right].Note)
				if overlap < OverlapThreshold {
					continue
				}
				analysis.Groups = append(analysis.Groups, OptimizationGroup{Scope: scoped[left].Scope, Entries: []Entry{scoped[left], scoped[right]}, Reason: "high semantic overlap candidate"})
				leftBytes, rightBytes := len([]byte(scoped[left].Note)), len([]byte(scoped[right].Note))
				if leftBytes < rightBytes {
					analysis.CandidateSavingsBytes += leftBytes / 2
				} else {
					analysis.CandidateSavingsBytes += rightBytes / 2
				}
			}
		}
	}
	analysis.OptimizationRecommended = len(analysis.Groups) > 0
	return analysis
}

func CompactionRecommended(entries []Entry, memoryBytes int) bool {
	if memoryBytes > MemorySoftLimitBytes {
		return true
	}
	counts := map[string]int{}
	for _, entry := range entries {
		id := strings.ToLower(normalizeName(entry.Scope))
		counts[id]++
		if counts[id] > ScopeEntriesSoftLimit {
			return true
		}
	}
	analysis := AnalyzeEntries(entries)
	for _, group := range analysis.Groups {
		if group.Reason == "high semantic overlap candidate" {
			return true
		}
	}
	return false
}

func tokenOverlap(left, right string) float64 {
	a, b := tokenSet(left), tokenSet(right)
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	union := map[string]struct{}{}
	for token := range a {
		union[token] = struct{}{}
		if _, ok := b[token]; ok {
			intersection++
		}
	}
	for token := range b {
		union[token] = struct{}{}
	}
	return float64(intersection) / float64(len(union))
}
