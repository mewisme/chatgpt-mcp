package cluster

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type RPCHandler func(context.Context, string, json.RawMessage) (json.RawMessage, error)
type AdvertisementProvider func() (Advertisement, error)

type Node struct {
	transport Transport
	handler   RPCHandler
	mu        sync.RWMutex
	advert    Advertisement
	provider  AdvertisementProvider
	session   Session
	cancel    context.CancelFunc
	done      chan struct{}
	pending   map[string]chan RPCResponse
	heartbeat time.Duration
}

func NewNode(transport Transport, advertisement Advertisement, handler RPCHandler) *Node {
	return &Node{transport: transport, advert: advertisement, handler: handler, pending: map[string]chan RPCResponse{}, heartbeat: 5 * time.Second}
}

func (n *Node) SetAdvertisementProvider(provider AdvertisementProvider) {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.provider = provider
	n.mu.Unlock()
}

func (n *Node) Start(ctx context.Context) error {
	if n == nil || n.transport == nil {
		return errors.New("cluster transport is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	advertisement, err := n.currentAdvertisement()
	if err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.session != nil {
		return errors.New("cluster node is already running")
	}
	session, err := n.transport.Connect(ctx, advertisement)
	if err != nil {
		return err
	}
	n.advert = advertisement
	runCtx, cancel := context.WithCancel(ctx)
	n.session = session
	n.cancel = cancel
	n.done = make(chan struct{})
	go n.run(runCtx, session, n.done)
	return nil
}

func (n *Node) Close() error {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	session, cancel, done := n.session, n.cancel, n.done
	n.session, n.cancel, n.done = nil, nil, nil
	n.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if session != nil {
		_ = session.Close()
	}
	if done != nil {
		<-done
	}
	n.failPending(ErrClosed)
	return nil
}

func (n *Node) Update(ctx context.Context, advertisement Advertisement) error {
	if err := validateAdvertisement(advertisement); err != nil {
		return err
	}
	n.mu.RLock()
	session := n.session
	n.mu.RUnlock()
	if session == nil {
		return ErrClosed
	}
	if err := session.Advertise(ctx, advertisement); err != nil {
		return err
	}
	n.mu.Lock()
	n.advert = advertisement
	n.mu.Unlock()
	return nil
}

func (n *Node) Snapshot(ctx context.Context) (Snapshot, error) {
	n.mu.RLock()
	session := n.session
	n.mu.RUnlock()
	if session == nil {
		return Snapshot{}, ErrClosed
	}
	return session.Snapshot(ctx)
}

func (n *Node) TryAcquireLeadership(ctx context.Context, tunnelID string, ttl time.Duration) (LeaderLease, bool, error) {
	n.mu.RLock()
	session := n.session
	n.mu.RUnlock()
	if session == nil {
		return LeaderLease{}, false, ErrClosed
	}
	return session.TryAcquireLeadership(ctx, tunnelID, ttl)
}

func (n *Node) RenewLeadership(ctx context.Context, lease LeaderLease, ttl time.Duration) (LeaderLease, error) {
	n.mu.RLock()
	session := n.session
	n.mu.RUnlock()
	if session == nil {
		return LeaderLease{}, ErrClosed
	}
	return session.RenewLeadership(ctx, lease, ttl)
}

func (n *Node) ReleaseLeadership(ctx context.Context, lease LeaderLease) error {
	n.mu.RLock()
	session := n.session
	n.mu.RUnlock()
	if session == nil {
		return ErrClosed
	}
	return session.ReleaseLeadership(ctx, lease)
}

func (n *Node) Leadership(ctx context.Context, tunnelID string) (LeaderLease, bool, error) {
	n.mu.RLock()
	session := n.session
	n.mu.RUnlock()
	if session == nil {
		return LeaderLease{}, false, ErrClosed
	}
	return session.Leadership(ctx, tunnelID)
}

func (n *Node) Advertisement() Advertisement {
	if n == nil {
		return Advertisement{}
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	value := n.advert
	value.Workspaces = append([]string(nil), value.Workspaces...)
	return value
}

func (n *Node) currentAdvertisement() (Advertisement, error) {
	n.mu.RLock()
	provider, advertisement := n.provider, n.advert
	n.mu.RUnlock()
	if provider == nil {
		return advertisement, validateAdvertisement(advertisement)
	}
	value, err := provider()
	if err != nil {
		return Advertisement{}, err
	}
	if err := validateAdvertisement(value); err != nil {
		return Advertisement{}, err
	}
	return value, nil
}

func (n *Node) WorkspaceOwner(ctx context.Context, workspaceID string) (WorkspaceOwner, error) {
	snapshot, err := n.Snapshot(ctx)
	if err != nil {
		return WorkspaceOwner{}, err
	}
	for _, owner := range snapshot.Workspaces {
		if owner.WorkspaceID != workspaceID {
			continue
		}
		if !owner.Online {
			return owner, fmt.Errorf("%w: %s", ErrOwnerOffline, owner.InstanceID)
		}
		return owner, nil
	}
	return WorkspaceOwner{}, fmt.Errorf("%w: %s", ErrNoOwner, workspaceID)
}

func (n *Node) Call(ctx context.Context, targetInstanceID, method string, payload json.RawMessage) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if targetInstanceID == "" {
		return nil, errors.New("cluster RPC target instance_id is required")
	}
	if method == "" {
		return nil, errors.New("cluster RPC method is required")
	}
	n.mu.RLock()
	session := n.session
	n.mu.RUnlock()
	if session == nil {
		return nil, ErrClosed
	}
	requestID, err := rpcID()
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(RPCRequest{Method: method, Payload: payload})
	if err != nil {
		return nil, err
	}
	responseCh := make(chan RPCResponse, 1)
	n.mu.Lock()
	n.pending[requestID] = responseCh
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		delete(n.pending, requestID)
		n.mu.Unlock()
	}()
	if err := session.Send(ctx, Frame{Kind: FrameRPCRequest, ToInstanceID: targetInstanceID, RequestID: requestID, Payload: encoded}); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response := <-responseCh:
		if response.Error != "" {
			return nil, errors.New(response.Error)
		}
		return response.Payload, nil
	}
}

func (n *Node) run(ctx context.Context, session Session, done chan struct{}) {
	defer close(done)
	heartbeat := n.heartbeat
	if heartbeat <= 0 {
		heartbeat = 5 * time.Second
	}
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		receiveCtx, cancel := context.WithCancel(ctx)
		frames := make(chan Frame, 1)
		errorsCh := make(chan error, 1)
		go func() {
			frame, err := session.Receive(receiveCtx)
			if err != nil {
				errorsCh <- err
				return
			}
			frames <- frame
		}()
		select {
		case <-ctx.Done():
			cancel()
			return
		case <-ticker.C:
			cancel()
			advertisement, err := n.currentAdvertisement()
			if err != nil {
				n.failPending(err)
				return
			}
			if err := session.Advertise(ctx, advertisement); err != nil {
				n.failPending(err)
				return
			}
			n.mu.Lock()
			n.advert = advertisement
			n.mu.Unlock()
		case err := <-errorsCh:
			cancel()
			if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrClosed) {
				n.failPending(err)
			}
			return
		case frame := <-frames:
			cancel()
			n.handleFrame(ctx, session, frame)
		}
	}
}

func (n *Node) handleFrame(ctx context.Context, session Session, frame Frame) {
	if frame.Version != ProtocolVersion {
		return
	}
	switch frame.Kind {
	case FrameRPCResponse:
		var response RPCResponse
		if json.Unmarshal(frame.Payload, &response) != nil {
			return
		}
		n.mu.RLock()
		pending := n.pending[frame.RequestID]
		n.mu.RUnlock()
		if pending != nil {
			select {
			case pending <- response:
			default:
			}
		}
	case FrameRPCRequest:
		go n.handleRequest(ctx, session, frame)
	}
}

func (n *Node) handleRequest(ctx context.Context, session Session, frame Frame) {
	var request RPCRequest
	response := RPCResponse{}
	if err := json.Unmarshal(frame.Payload, &request); err != nil {
		response.Error = "decode cluster RPC request: " + err.Error()
	} else if n.handler == nil {
		response.Error = "cluster RPC handler is unavailable"
	} else {
		payload, err := n.handler(ctx, request.Method, request.Payload)
		if err != nil {
			response.Error = err.Error()
		} else {
			response.Payload = payload
		}
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return
	}
	_ = session.Send(ctx, Frame{Kind: FrameRPCResponse, ToInstanceID: frame.FromInstanceID, RequestID: frame.RequestID, Payload: encoded})
}

func (n *Node) failPending(err error) {
	if err == nil {
		err = ErrClosed
	}
	n.mu.Lock()
	pending := make([]chan RPCResponse, 0, len(n.pending))
	for id, channel := range n.pending {
		pending = append(pending, channel)
		delete(n.pending, id)
	}
	n.mu.Unlock()
	for _, channel := range pending {
		select {
		case channel <- RPCResponse{Error: err.Error()}:
		default:
		}
	}
}

func rpcID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "rpc_" + hex.EncodeToString(raw[:]), nil
}
