package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	toolLoopHistoryLimit = 32
	toolLoopSessionTTL   = 30 * time.Minute
)

type toolLoopClass string

const (
	toolLoopClassContext  toolLoopClass = "context"
	toolLoopClassRead     toolLoopClass = "read"
	toolLoopClassMutation toolLoopClass = "mutation"
	toolLoopClassExempt   toolLoopClass = "exempt"
)

type ToolLoopGuard struct {
	mu       sync.Mutex
	sessions map[string]*toolLoopSession
	now      func() time.Time
}

type toolLoopSession struct {
	history  []toolLoopRecord
	progress uint64
	lastSeen time.Time
}

type toolLoopRecord struct {
	tool        string
	fingerprint string
	progress    uint64
}

type toolLoopDecision struct {
	blocked bool
	warn    bool
	reason  string
	repeats int
}

func NewToolLoopGuard() *ToolLoopGuard {
	return &ToolLoopGuard{sessions: map[string]*toolLoopSession{}, now: time.Now}
}

func (g *ToolLoopGuard) Check(sessionID, name string, args map[string]any, class toolLoopClass) toolLoopDecision {
	if g == nil || strings.TrimSpace(sessionID) == "" || class == toolLoopClassExempt || class == toolLoopClassMutation {
		return toolLoopDecision{}
	}
	fingerprint := toolCallFingerprint(name, args)
	now := g.clock()()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.purgeLocked(now)
	session := g.sessionLocked(sessionID, now)
	record := toolLoopRecord{tool: name, fingerprint: fingerprint, progress: session.progress}
	consecutive := consecutiveFingerprintCount(session.history, record) + 1
	exactLimit := 4
	if class == toolLoopClassContext {
		exactLimit = 3
	}
	decision := toolLoopDecision{warn: consecutive == exactLimit-1, repeats: consecutive}
	if consecutive >= exactLimit {
		decision.blocked = true
		decision.reason = "exact_duplicate"
		return decision
	}
	prospective := append(append([]toolLoopRecord(nil), session.history...), record)
	if cycleLength, ok := repeatedTailCycle(prospective, session.progress); ok {
		decision.blocked = true
		decision.reason = fmt.Sprintf("repeated_cycle_%d", cycleLength)
		decision.repeats = 3
		return decision
	}
	session.history = append(session.history, record)
	if len(session.history) > toolLoopHistoryLimit {
		session.history = append([]toolLoopRecord(nil), session.history[len(session.history)-toolLoopHistoryLimit:]...)
	}
	return decision
}

func (g *ToolLoopGuard) MarkProgress(sessionID string) {
	if g == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	now := g.clock()()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.purgeLocked(now)
	session := g.sessionLocked(sessionID, now)
	session.progress++
	session.history = nil
}

func (g *ToolLoopGuard) Delete(sessionID string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	delete(g.sessions, strings.TrimSpace(sessionID))
	g.mu.Unlock()
}

func (g *ToolLoopGuard) clock() func() time.Time {
	if g.now != nil {
		return g.now
	}
	return time.Now
}

func (g *ToolLoopGuard) sessionLocked(sessionID string, now time.Time) *toolLoopSession {
	sessionID = strings.TrimSpace(sessionID)
	session := g.sessions[sessionID]
	if session == nil {
		session = &toolLoopSession{}
		g.sessions[sessionID] = session
	}
	session.lastSeen = now
	return session
}

func (g *ToolLoopGuard) purgeLocked(now time.Time) {
	for id, session := range g.sessions {
		if session == nil || now.Sub(session.lastSeen) >= toolLoopSessionTTL {
			delete(g.sessions, id)
		}
	}
}

func consecutiveFingerprintCount(history []toolLoopRecord, record toolLoopRecord) int {
	count := 0
	for index := len(history) - 1; index >= 0; index-- {
		item := history[index]
		if item.progress != record.progress || item.fingerprint != record.fingerprint {
			break
		}
		count++
	}
	return count
}

func repeatedTailCycle(history []toolLoopRecord, progress uint64) (int, bool) {
	start := 0
	for index := len(history) - 1; index >= 0; index-- {
		if history[index].progress != progress {
			start = index + 1
			break
		}
	}
	history = history[start:]
	for cycleLength := 2; cycleLength <= 4; cycleLength++ {
		need := cycleLength * 3
		if len(history) < need {
			continue
		}
		tail := history[len(history)-need:]
		match := true
		for index := cycleLength; index < len(tail); index++ {
			if tail[index].fingerprint != tail[index%cycleLength].fingerprint {
				match = false
				break
			}
		}
		if match {
			return cycleLength, true
		}
	}
	return 0, false
}

func toolCallFingerprint(name string, args map[string]any) string {
	data, err := json.Marshal(args)
	if err != nil {
		data = []byte(fmt.Sprintf("%v", args))
	}
	sum := sha256.Sum256(append([]byte(strings.TrimSpace(name)+"\n"), data...))
	return hex.EncodeToString(sum[:])
}

func toolLoopClassFor(name string, schema Schema) toolLoopClass {
	switch strings.TrimSpace(name) {
	case "process_status", "process_output", "workspace_status", "shell_status", "agent_status", "get_version":
		return toolLoopClassExempt
	case "project_context", "load_path_rules", "list_skills", "load_skill":
		return toolLoopClassContext
	}
	if readOnly, _ := schema.Annotations["readOnlyHint"].(bool); readOnly {
		return toolLoopClassRead
	}
	return toolLoopClassMutation
}

func toolLoopBlockedResult(name string, decision toolLoopDecision) Result {
	message := fmt.Sprintf("Tool loop detected: %s is repeating without an intervening state-changing action. Reuse the previous result or choose a different action.", name)
	result := ErrorResult(fmt.Errorf("%s", message))
	result.Meta = map[string]any{"loopGuard": map[string]any{"blocked": true, "reason": decision.reason, "repeats": decision.repeats, "tool": name}}
	return result
}

func addToolLoopWarning(result Result, name string, decision toolLoopDecision) Result {
	if !decision.warn || decision.blocked {
		return result
	}
	if result.Meta == nil {
		result.Meta = map[string]any{}
	} else {
		result.Meta = cloneMap(result.Meta)
	}
	result.Meta["loopGuard"] = map[string]any{"warning": true, "repeats": decision.repeats, "tool": name}
	return result
}
