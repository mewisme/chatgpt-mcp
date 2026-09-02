package cluster

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemoryRelay struct {
	mu        sync.RWMutex
	members   map[string]Member
	owners    map[string]string
	leaders   map[string]LeaderLease
	epochs    map[string]uint64
	sessions  map[string]*memorySession
	inboxSize int
	now       func() time.Time
}

func NewMemoryRelay() *MemoryRelay {
	return &MemoryRelay{members: map[string]Member{}, owners: map[string]string{}, leaders: map[string]LeaderLease{}, epochs: map[string]uint64{}, sessions: map[string]*memorySession{}, inboxSize: 128, now: func() time.Time { return time.Now().UTC() }}
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLeadersLocked()
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
	leaders := make([]LeaderLease, 0, len(r.leaders))
	for _, lease := range r.leaders {
		leaders = append(leaders, lease)
	}
	sort.Slice(leaders, func(i, j int) bool { return leaders[i].TunnelID < leaders[j].TunnelID })
	catalogHash, compatible, catalogError := catalogStatus(members)
	return Snapshot{Members: members, Workspaces: workspaces, Leaders: leaders, CatalogHash: catalogHash, CatalogCompatible: compatible, CatalogError: catalogError}
}

func (r *MemoryRelay) tryAcquireLeadership(instanceID, tunnelID string, ttl time.Duration) (LeaderLease, bool, error) {
	tunnelID = strings.TrimSpace(tunnelID)
	if tunnelID == "" {
		return LeaderLease{}, false, errors.New("cluster leader tunnel_id is required")
	}
	if ttl <= 0 {
		return LeaderLease{}, false, errors.New("cluster leader lease ttl must be greater than zero")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateSessionLocked(instanceID); err != nil {
		return LeaderLease{}, false, err
	}
	r.expireLeadersLocked()
	if current, ok := r.leaders[tunnelID]; ok {
		if current.InstanceID != instanceID {
			return current, false, nil
		}
		current.ExpiresAt = r.now().Add(ttl)
		r.leaders[tunnelID] = current
		return current, true, nil
	}
	r.epochs[tunnelID]++
	lease := LeaderLease{TunnelID: tunnelID, InstanceID: instanceID, Epoch: r.epochs[tunnelID], ExpiresAt: r.now().Add(ttl)}
	r.leaders[tunnelID] = lease
	return lease, true, nil
}

func (r *MemoryRelay) renewLeadership(instanceID string, lease LeaderLease, ttl time.Duration) (LeaderLease, error) {
	if ttl <= 0 {
		return LeaderLease{}, errors.New("cluster leader lease ttl must be greater than zero")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateSessionLocked(instanceID); err != nil {
		return LeaderLease{}, err
	}
	r.expireLeadersLocked()
	current, ok := r.leaders[lease.TunnelID]
	if !ok || current.InstanceID != instanceID || current.InstanceID != lease.InstanceID || current.Epoch != lease.Epoch {
		return LeaderLease{}, ErrLeaseLost
	}
	current.ExpiresAt = r.now().Add(ttl)
	r.leaders[lease.TunnelID] = current
	return current, nil
}

func (r *MemoryRelay) releaseLeadership(instanceID string, lease LeaderLease) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateSessionLocked(instanceID); err != nil {
		return err
	}
	r.expireLeadersLocked()
	current, ok := r.leaders[lease.TunnelID]
	if !ok || current.InstanceID != instanceID || current.InstanceID != lease.InstanceID || current.Epoch != lease.Epoch {
		return ErrLeaseLost
	}
	delete(r.leaders, lease.TunnelID)
	return nil
}

func (r *MemoryRelay) leadership(instanceID, tunnelID string) (LeaderLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateSessionLocked(instanceID); err != nil {
		return LeaderLease{}, false, err
	}
	r.expireLeadersLocked()
	lease, ok := r.leaders[strings.TrimSpace(tunnelID)]
	return lease, ok, nil
}

func (r *MemoryRelay) validateSessionLocked(instanceID string) error {
	session := r.sessions[instanceID]
	if session == nil || session.isClosed() || !r.members[instanceID].Online {
		return ErrClosed
	}
	return nil
}

func (r *MemoryRelay) expireLeadersLocked() {
	now := r.now()
	for tunnelID, lease := range r.leaders {
		if !r.members[lease.InstanceID].Online || !lease.ExpiresAt.After(now) {
			delete(r.leaders, tunnelID)
		}
	}
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

func (s *memorySession) TryAcquireLeadership(_ context.Context, tunnelID string, ttl time.Duration) (LeaderLease, bool, error) {
	if s.isClosed() {
		return LeaderLease{}, false, ErrClosed
	}
	return s.relay.tryAcquireLeadership(s.instanceID, tunnelID, ttl)
}

func (s *memorySession) RenewLeadership(_ context.Context, lease LeaderLease, ttl time.Duration) (LeaderLease, error) {
	if s.isClosed() {
		return LeaderLease{}, ErrClosed
	}
	return s.relay.renewLeadership(s.instanceID, lease, ttl)
}

func (s *memorySession) ReleaseLeadership(_ context.Context, lease LeaderLease) error {
	if s.isClosed() {
		return ErrClosed
	}
	return s.relay.releaseLeadership(s.instanceID, lease)
}

func (s *memorySession) Leadership(_ context.Context, tunnelID string) (LeaderLease, bool, error) {
	if s.isClosed() {
		return LeaderLease{}, false, ErrClosed
	}
	return s.relay.leadership(s.instanceID, tunnelID)
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
