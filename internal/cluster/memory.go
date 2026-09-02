package cluster

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

type MemoryRelay struct {
	mu        sync.RWMutex
	members   map[string]Member
	owners    map[string]string
	sessions  map[string]*memorySession
	inboxSize int
	now       func() time.Time
}

func NewMemoryRelay() *MemoryRelay {
	return &MemoryRelay{members: map[string]Member{}, owners: map[string]string{}, sessions: map[string]*memorySession{}, inboxSize: 128, now: func() time.Time { return time.Now().UTC() }}
}

func (r *MemoryRelay) Connect(ctx context.Context, advertisement Advertisement) (Session, error) {
	if err := validateAdvertisement(advertisement); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing := r.sessions[advertisement.InstanceID]; existing != nil && !existing.isClosed() {
		return nil, fmt.Errorf("cluster instance is already connected: %s", advertisement.InstanceID)
	}
	if err := r.validateOwnershipLocked(advertisement.InstanceID, advertisement.Workspaces); err != nil {
		return nil, err
	}
	now := r.now()
	member := r.members[advertisement.InstanceID]
	member.InstanceID = advertisement.InstanceID
	member.Name = advertisement.Name
	member.CatalogHash = advertisement.CatalogHash
	member.Workspaces = normalizeWorkspaces(advertisement.Workspaces)
	member.Online = true
	member.ConnectedAt = now
	member.LastSeen = now
	r.members[member.InstanceID] = member
	r.replaceOwnershipLocked(member.InstanceID, member.Workspaces)
	sessionCtx, cancel := context.WithCancel(ctx)
	session := &memorySession{relay: r, instanceID: member.InstanceID, inbox: make(chan Frame, r.inboxSize), ctx: sessionCtx, cancel: cancel}
	r.sessions[member.InstanceID] = session
	go func() {
		<-sessionCtx.Done()
		_ = session.Close()
	}()
	return session, nil
}

func (r *MemoryRelay) validateOwnershipLocked(instanceID string, workspaces []string) error {
	for _, workspaceID := range normalizeWorkspaces(workspaces) {
		if owner := r.owners[workspaceID]; owner != "" && owner != instanceID {
			return fmt.Errorf("workspace %s is already owned by instance %s", workspaceID, owner)
		}
	}
	return nil
}

func (r *MemoryRelay) replaceOwnershipLocked(instanceID string, workspaces []string) {
	advertised := map[string]bool{}
	for _, workspaceID := range normalizeWorkspaces(workspaces) {
		advertised[workspaceID] = true
		r.owners[workspaceID] = instanceID
	}
	for workspaceID, owner := range r.owners {
		if owner == instanceID && !advertised[workspaceID] {
			delete(r.owners, workspaceID)
		}
	}
}

func (r *MemoryRelay) advertise(instanceID string, advertisement Advertisement) error {
	if err := validateAdvertisement(advertisement); err != nil {
		return err
	}
	if advertisement.InstanceID != instanceID {
		return errors.New("cluster advertisement instance_id does not match session")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[instanceID]
	if session == nil || session.isClosed() {
		return ErrClosed
	}
	if err := r.validateOwnershipLocked(instanceID, advertisement.Workspaces); err != nil {
		return err
	}
	member := r.members[instanceID]
	member.Name = advertisement.Name
	member.CatalogHash = advertisement.CatalogHash
	member.Workspaces = normalizeWorkspaces(advertisement.Workspaces)
	member.Online = true
	member.LastSeen = r.now()
	r.members[instanceID] = member
	r.replaceOwnershipLocked(instanceID, member.Workspaces)
	return nil
}

func (r *MemoryRelay) snapshot() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	members := make([]Member, 0, len(r.members))
	for _, member := range r.members {
		member.Workspaces = append([]string(nil), member.Workspaces...)
		members = append(members, member)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].InstanceID < members[j].InstanceID })
	workspaces := make([]WorkspaceOwner, 0, len(r.owners))
	for workspaceID, instanceID := range r.owners {
		member := r.members[instanceID]
		workspaces = append(workspaces, WorkspaceOwner{WorkspaceID: workspaceID, InstanceID: instanceID, Online: member.Online})
	}
	sort.Slice(workspaces, func(i, j int) bool { return workspaces[i].WorkspaceID < workspaces[j].WorkspaceID })
	return Snapshot{Members: members, Workspaces: workspaces}
}

func (r *MemoryRelay) send(ctx context.Context, sender string, frame Frame) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if frame.ToInstanceID == "" {
		return errors.New("cluster frame target instance_id is required")
	}
	if frame.RequestID == "" {
		return errors.New("cluster frame request_id is required")
	}
	if frame.Kind != FrameRPCRequest && frame.Kind != FrameRPCResponse {
		return fmt.Errorf("unsupported cluster frame kind: %s", frame.Kind)
	}
	r.mu.Lock()
	senderSession := r.sessions[sender]
	if senderSession == nil || senderSession.isClosed() {
		r.mu.Unlock()
		return ErrClosed
	}
	senderMember := r.members[sender]
	senderMember.LastSeen = r.now()
	r.members[sender] = senderMember
	target := r.sessions[frame.ToInstanceID]
	targetMember := r.members[frame.ToInstanceID]
	r.mu.Unlock()
	if target == nil || target.isClosed() || !targetMember.Online {
		return fmt.Errorf("cluster target instance is offline: %s", frame.ToInstanceID)
	}
	frame.Version = ProtocolVersion
	frame.FromInstanceID = sender
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-target.ctx.Done():
		return fmt.Errorf("cluster target instance is offline: %s", frame.ToInstanceID)
	case target.inbox <- frame:
		return nil
	}
}

func (r *MemoryRelay) close(instanceID string, session *memorySession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions[instanceID] != session {
		return
	}
	delete(r.sessions, instanceID)
	member := r.members[instanceID]
	member.Online = false
	member.LastSeen = r.now()
	r.members[instanceID] = member
}

type memorySession struct {
	relay      *MemoryRelay
	instanceID string
	inbox      chan Frame
	ctx        context.Context
	cancel     context.CancelFunc
	closeOnce  sync.Once
}

func (s *memorySession) Advertise(_ context.Context, advertisement Advertisement) error {
	if s.isClosed() {
		return ErrClosed
	}
	return s.relay.advertise(s.instanceID, advertisement)
}

func (s *memorySession) Snapshot(context.Context) (Snapshot, error) {
	if s.isClosed() {
		return Snapshot{}, ErrClosed
	}
	return s.relay.snapshot(), nil
}

func (s *memorySession) Send(ctx context.Context, frame Frame) error {
	if s.isClosed() {
		return ErrClosed
	}
	return s.relay.send(ctx, s.instanceID, frame)
}

func (s *memorySession) Receive(ctx context.Context) (Frame, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return Frame{}, ctx.Err()
	case <-s.ctx.Done():
		return Frame{}, ErrClosed
	case frame := <-s.inbox:
		return frame, nil
	}
}

func (s *memorySession) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		s.relay.close(s.instanceID, s)
	})
	return nil
}

func (s *memorySession) isClosed() bool {
	select {
	case <-s.ctx.Done():
		return true
	default:
		return false
	}
}
