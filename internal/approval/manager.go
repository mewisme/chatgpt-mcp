package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type challengeRecord struct {
	value Challenge
}

type requestRecord struct {
	value    Request
	resolved chan struct{}
	closed   bool
}

type Manager struct {
	mu                    sync.Mutex
	instanceID            string
	challenges            map[string]*challengeRecord
	challengeByTarget     map[string]string
	requests              map[string]*requestRecord
	activeBySession       map[string]string
	now                   func() time.Time
	newID                 func(string) (string, error)
	challengeTTL          time.Duration
	requestTTL            time.Duration
	retryTTL              time.Duration
	pendingLimit          int
	workspacePendingLimit int
}

func NewManager(instanceID string) *Manager {
	return &Manager{
		instanceID: strings.TrimSpace(instanceID), challenges: map[string]*challengeRecord{}, challengeByTarget: map[string]string{}, requests: map[string]*requestRecord{}, activeBySession: map[string]string{},
		now: time.Now, newID: randomID, challengeTTL: DefaultChallengeTTL, requestTTL: DefaultRequestTTL, retryTTL: DefaultRetryTTL, pendingLimit: DefaultPendingLimit, workspacePendingLimit: DefaultWorkspacePendingLimit,
	}
}

func (m *Manager) CreateChallenge(input ChallengeInput) (Challenge, bool, error) {
	if m == nil {
		return Challenge{}, false, errors.New("approval manager is unavailable")
	}
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.SessionHash = strings.TrimSpace(input.SessionHash)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.Source = strings.TrimSpace(input.Source)
	input.TargetTool = strings.TrimSpace(input.TargetTool)
	input.GuardReason = strings.TrimSpace(input.GuardReason)
	input.Title = strings.TrimSpace(input.Title)
	digest, arguments, err := CanonicalTargetDigest(m.instanceID, Target{SessionID: input.SessionID, WorkspaceID: input.WorkspaceID, Source: input.Source, TargetTool: input.TargetTool, Arguments: input.Arguments, GuardCode: input.GuardCode})
	if err != nil {
		return Challenge{}, false, err
	}
	if input.Title == "" {
		input.Title = "Allow " + input.TargetTool
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().UTC()
	m.purgeExpiredLocked(now)
	key := challengeTargetKey(input.SessionID, digest)
	if id := m.challengeByTarget[key]; id != "" {
		if record := m.challenges[id]; record != nil {
			if request := m.requests[record.value.requestID]; request == nil || request.value.Status == StatusPending || request.value.Status == StatusApproved {
				record.value.ExpiresAt = now.Add(m.challengeTTL)
				return cloneChallenge(record.value), false, nil
			}
		}
		delete(m.challengeByTarget, key)
	}
	id, err := m.newID("chg")
	if err != nil {
		return Challenge{}, false, err
	}
	value := Challenge{
		ID: id, SessionHash: input.SessionHash, WorkspaceID: input.WorkspaceID, Source: input.Source, TargetTool: input.TargetTool, Arguments: arguments, Digest: digest,
		GuardCode: input.GuardCode, GuardReason: input.GuardReason, Title: input.Title, CreatedAt: now, ExpiresAt: now.Add(m.challengeTTL), sessionID: input.SessionID,
	}
	m.challenges[id] = &challengeRecord{value: value}
	m.challengeByTarget[key] = id
	return cloneChallenge(value), true, nil
}

func (m *Manager) CreateRequest(challengeID, sessionID, workspaceID string) (Request, bool, error) {
	if m == nil {
		return Request{}, false, errors.New("approval manager is unavailable")
	}
	challengeID, sessionID, workspaceID = strings.TrimSpace(challengeID), strings.TrimSpace(sessionID), strings.TrimSpace(workspaceID)
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().UTC()
	challenge := m.challenges[challengeID]
	if challenge == nil {
		m.purgeExpiredLocked(now)
		return Request{}, false, ErrChallengeNotFound
	}
	if !now.Before(challenge.value.ExpiresAt) {
		m.removeChallengeLocked(challenge.value)
		m.purgeExpiredLocked(now)
		return Request{}, false, ErrChallengeExpired
	}
	m.purgeExpiredLocked(now)
	challenge = m.challenges[challengeID]
	if challenge == nil {
		return Request{}, false, ErrChallengeNotFound
	}
	if challenge.value.sessionID != sessionID || challenge.value.WorkspaceID != workspaceID {
		return Request{}, false, ErrChallengeMismatch
	}
	if challenge.value.requestID != "" {
		if request := m.requests[challenge.value.requestID]; request != nil {
			return cloneRequest(request.value), false, nil
		}
	}
	if activeID := m.activeBySession[sessionID]; activeID != "" {
		if active := m.requests[activeID]; active != nil && (active.value.Status == StatusPending || active.value.Status == StatusApproved) {
			if active.value.Digest != challenge.value.Digest {
				return Request{}, false, ErrSessionRequestActive
			}
			challenge.value.requestID = active.value.ID
			return cloneRequest(active.value), false, nil
		}
		delete(m.activeBySession, sessionID)
	}
	if m.pendingLimit > 0 && m.pendingCountLocked("") >= m.pendingLimit {
		return Request{}, false, ErrPendingLimit
	}
	if m.workspacePendingLimit > 0 && m.pendingCountLocked(workspaceID) >= m.workspacePendingLimit {
		return Request{}, false, fmt.Errorf("%w for workspace %s", ErrPendingLimit, workspaceID)
	}
	id, err := m.newID("req")
	if err != nil {
		return Request{}, false, err
	}
	value := Request{
		ID: id, Status: StatusPending, WorkspaceID: challenge.value.WorkspaceID, SessionHash: challenge.value.SessionHash, Source: challenge.value.Source, TargetTool: challenge.value.TargetTool,
		Arguments: cloneRaw(challenge.value.Arguments), Digest: challenge.value.Digest, GuardCode: challenge.value.GuardCode, GuardReason: challenge.value.GuardReason, Title: challenge.value.Title,
		CreatedAt: now, ExpiresAt: now.Add(m.requestTTL), sessionID: sessionID, challengeID: challenge.value.ID,
	}
	m.requests[id] = &requestRecord{value: value, resolved: make(chan struct{})}
	m.activeBySession[sessionID] = id
	challenge.value.requestID = id
	return cloneRequest(value), true, nil
}

func (m *Manager) Approve(id, resolvedBy, reason string) (Request, error) {
	return m.resolve(id, StatusApproved, resolvedBy, reason)
}

func (m *Manager) Deny(id, resolvedBy, reason string) (Request, error) {
	return m.resolve(id, StatusDenied, resolvedBy, reason)
}

func (m *Manager) Cancel(id, resolvedBy, reason string) (Request, error) {
	return m.resolve(id, StatusCancelled, resolvedBy, reason)
}

func (m *Manager) resolve(id string, status Status, resolvedBy, reason string) (Request, error) {
	if m == nil {
		return Request{}, errors.New("approval manager is unavailable")
	}
	id, resolvedBy, reason = strings.TrimSpace(id), strings.TrimSpace(resolvedBy), strings.TrimSpace(reason)
	if status != StatusApproved && status != StatusDenied && status != StatusCancelled {
		return Request{}, fmt.Errorf("unsupported approval resolution status: %s", status)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().UTC()
	m.purgeExpiredLocked(now)
	record := m.requests[id]
	if record == nil {
		return Request{}, ErrRequestNotFound
	}
	if record.value.Status == status {
		return cloneRequest(record.value), nil
	}
	if record.value.Status != StatusPending {
		return Request{}, fmt.Errorf("%w: %s", ErrRequestResolved, record.value.Status)
	}
	record.value.Status, record.value.ResolvedAt, record.value.ResolvedBy, record.value.Reason = status, now, resolvedBy, reason
	if status == StatusApproved {
		record.value.RetryUntil = now.Add(m.retryTTL)
	} else {
		m.clearActiveLocked(record.value)
	}
	m.closeResolvedLocked(record)
	return cloneRequest(record.value), nil
}

func (m *Manager) Wait(ctx context.Context, id string) (Request, error) {
	if m == nil {
		return Request{}, errors.New("approval manager is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	now := m.now().UTC()
	m.purgeExpiredLocked(now)
	record := m.requests[strings.TrimSpace(id)]
	if record == nil {
		m.mu.Unlock()
		return Request{}, ErrRequestNotFound
	}
	if record.value.Status != StatusPending {
		value := cloneRequest(record.value)
		m.mu.Unlock()
		return value, nil
	}
	resolved, expiresAt := record.resolved, record.value.ExpiresAt
	m.mu.Unlock()
	duration := expiresAt.Sub(now)
	if duration < 0 {
		duration = 0
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-resolved:
		value, ok := m.Get(id)
		if !ok {
			return Request{}, ErrRequestNotFound
		}
		return value, nil
	case <-timer.C:
		m.PurgeExpired()
		value, ok := m.Get(id)
		if !ok {
			return Request{}, ErrRequestNotFound
		}
		return value, nil
	case <-ctx.Done():
		value, err := m.Cancel(id, "mcp", "approval wait cancelled")
		if err != nil && !errors.Is(err, ErrRequestResolved) {
			return Request{}, err
		}
		return value, ctx.Err()
	}
}

func (m *Manager) MatchApproved(input RetryInput) (Request, bool, error) {
	if m == nil {
		return Request{}, false, errors.New("approval manager is unavailable")
	}
	input.SessionID, input.WorkspaceID, input.Source, input.TargetTool = strings.TrimSpace(input.SessionID), strings.TrimSpace(input.WorkspaceID), strings.TrimSpace(input.Source), strings.TrimSpace(input.TargetTool)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeExpiredLocked(m.now().UTC())
	return m.matchApprovedLocked(input)
}

func (m *Manager) ClaimApproved(input RetryInput) (Request, bool, error) {
	if m == nil {
		return Request{}, false, errors.New("approval manager is unavailable")
	}
	input.SessionID, input.WorkspaceID, input.Source, input.TargetTool = strings.TrimSpace(input.SessionID), strings.TrimSpace(input.WorkspaceID), strings.TrimSpace(input.Source), strings.TrimSpace(input.TargetTool)
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().UTC()
	m.purgeExpiredLocked(now)
	request, matched, err := m.matchApprovedLocked(input)
	if err != nil || !matched {
		return request, matched, err
	}
	record := m.requests[request.ID]
	if record == nil || record.value.Status != StatusApproved {
		return Request{}, false, ErrRequestNotApproved
	}
	record.value.Status, record.value.ConsumedAt = StatusConsumed, now
	m.clearActiveLocked(record.value)
	return cloneRequest(record.value), true, nil
}

func (m *Manager) matchApprovedLocked(input RetryInput) (Request, bool, error) {
	active := m.requests[m.activeBySession[input.SessionID]]
	if active == nil || active.value.Status != StatusApproved {
		return Request{}, false, nil
	}
	if active.value.TargetTool != input.TargetTool {
		return Request{}, false, nil
	}
	digest, actual, err := CanonicalTargetDigest(m.instanceID, Target{SessionID: input.SessionID, WorkspaceID: input.WorkspaceID, Source: input.Source, TargetTool: input.TargetTool, Arguments: input.Arguments, GuardCode: active.value.GuardCode})
	if err != nil {
		return Request{}, false, err
	}
	if digest != active.value.Digest {
		return Request{}, false, &MismatchError{RequestID: active.value.ID, TargetTool: active.value.TargetTool, Expected: cloneRaw(active.value.Arguments), Actual: actual}
	}
	return cloneRequest(active.value), true, nil
}

func (m *Manager) Consume(id string) (Request, error) {
	if m == nil {
		return Request{}, errors.New("approval manager is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().UTC()
	m.purgeExpiredLocked(now)
	record := m.requests[strings.TrimSpace(id)]
	if record == nil {
		return Request{}, ErrRequestNotFound
	}
	if record.value.Status != StatusApproved {
		return Request{}, fmt.Errorf("%w: %s", ErrRequestNotApproved, record.value.Status)
	}
	record.value.Status, record.value.ConsumedAt = StatusConsumed, now
	m.clearActiveLocked(record.value)
	return cloneRequest(record.value), nil
}

func (m *Manager) Get(id string) (Request, bool) {
	if m == nil {
		return Request{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeExpiredLocked(m.now().UTC())
	record := m.requests[strings.TrimSpace(id)]
	if record == nil {
		return Request{}, false
	}
	return cloneRequest(record.value), true
}

func (m *Manager) List(filter Filter) []Request {
	if m == nil {
		return nil
	}
	filter.WorkspaceID = strings.TrimSpace(filter.WorkspaceID)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeExpiredLocked(m.now().UTC())
	values := make([]Request, 0, len(m.requests))
	for _, record := range m.requests {
		if filter.WorkspaceID != "" && record.value.WorkspaceID != filter.WorkspaceID {
			continue
		}
		if filter.Status != "" && record.value.Status != filter.Status {
			continue
		}
		values = append(values, cloneRequest(record.value))
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].CreatedAt.Equal(values[j].CreatedAt) {
			return values[i].ID < values[j].ID
		}
		return values[i].CreatedAt.After(values[j].CreatedAt)
	})
	return values
}

func (m *Manager) PurgeExpired() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.purgeExpiredLocked(m.now().UTC())
}

func (m *Manager) purgeExpiredLocked(now time.Time) int {
	changed := 0
	for _, record := range m.requests {
		switch record.value.Status {
		case StatusPending:
			if now.Before(record.value.ExpiresAt) {
				continue
			}
			record.value.Status, record.value.ResolvedAt, record.value.Reason = StatusExpired, now, "approval request expired"
			m.clearActiveLocked(record.value)
			m.closeResolvedLocked(record)
			changed++
		case StatusApproved:
			if record.value.RetryUntil.IsZero() || now.Before(record.value.RetryUntil) {
				continue
			}
			record.value.Status, record.value.ResolvedAt, record.value.Reason = StatusExpired, now, "approved retry window expired"
			m.clearActiveLocked(record.value)
			changed++
		}
	}
	for _, record := range m.challenges {
		if now.Before(record.value.ExpiresAt) {
			continue
		}
		m.removeChallengeLocked(record.value)
		changed++
	}
	return changed
}

func (m *Manager) pendingCountLocked(workspaceID string) int {
	count := 0
	for _, record := range m.requests {
		if record.value.Status == StatusPending && (workspaceID == "" || record.value.WorkspaceID == workspaceID) {
			count++
		}
	}
	return count
}

func (m *Manager) clearActiveLocked(value Request) {
	if m.activeBySession[value.sessionID] == value.ID {
		delete(m.activeBySession, value.sessionID)
	}
}

func (m *Manager) closeResolvedLocked(record *requestRecord) {
	if record == nil || record.closed {
		return
	}
	close(record.resolved)
	record.closed = true
}

func (m *Manager) removeChallengeLocked(value Challenge) {
	delete(m.challenges, value.ID)
	key := challengeTargetKey(value.sessionID, value.Digest)
	if m.challengeByTarget[key] == value.ID {
		delete(m.challengeByTarget, key)
	}
}

func challengeTargetKey(sessionID, digest string) string { return sessionID + "\x00" + digest }

func cloneChallenge(value Challenge) Challenge {
	value.Arguments = cloneRaw(value.Arguments)
	return value
}

func cloneRequest(value Request) Request {
	value.Arguments = cloneRaw(value.Arguments)
	return value
}

func randomID(prefix string) (string, error) {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(buffer), nil
}
