package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	clusterSubprotocol = "chatgpt-mcp-cluster-v1"
	maxWireMessageSize = 4 << 20
)

type wireMessage struct {
	Version       int            `json:"version"`
	Kind          string         `json:"kind"`
	ID            string         `json:"id,omitempty"`
	Advertisement *Advertisement `json:"advertisement,omitempty"`
	Frame         *Frame         `json:"frame,omitempty"`
	Snapshot      *Snapshot      `json:"snapshot,omitempty"`
	Lease         *LeaderLease   `json:"lease,omitempty"`
	TunnelID      string         `json:"tunnel_id,omitempty"`
	TTLMillis     int64          `json:"ttl_ms,omitempty"`
	Acquired      bool           `json:"acquired,omitempty"`
	Found         bool           `json:"found,omitempty"`
	Error         string         `json:"error,omitempty"`
}

type WebSocketTransport struct {
	URL        string
	Token      string
	HTTPClient *http.Client
}

func NewWebSocketTransport(url, token string) *WebSocketTransport {
	return &WebSocketTransport{URL: strings.TrimSpace(url), Token: token}
}

func (t *WebSocketTransport) Connect(ctx context.Context, advertisement Advertisement) (Session, error) {
	if t == nil || strings.TrimSpace(t.URL) == "" {
		return nil, errors.New("cluster relay URL is required")
	}
	if strings.TrimSpace(t.Token) == "" {
		return nil, errors.New("cluster relay token is required")
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+t.Token)
	conn, response, err := websocket.Dial(ctx, t.URL, &websocket.DialOptions{HTTPClient: t.HTTPClient, HTTPHeader: header, Subprotocols: []string{clusterSubprotocol}})
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("connect cluster relay: HTTP %d: %w", response.StatusCode, err)
		}
		return nil, fmt.Errorf("connect cluster relay: %w", err)
	}
	if conn.Subprotocol() != clusterSubprotocol {
		_ = conn.Close(websocket.StatusPolicyViolation, "cluster subprotocol required")
		return nil, errors.New("cluster relay did not negotiate the required subprotocol")
	}
	conn.SetReadLimit(maxWireMessageSize)
	runCtx, cancel := context.WithCancel(context.Background())
	session := &webSocketSession{conn: conn, ctx: runCtx, cancel: cancel, inbox: make(chan Frame, 128), pending: map[string]chan wireMessage{}}
	go session.readLoop()
	responseMessage, err := session.request(ctx, wireMessage{Kind: "hello", Advertisement: &advertisement})
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	if responseMessage.Error != "" {
		_ = session.Close()
		return nil, errors.New(responseMessage.Error)
	}
	return session, nil
}

type webSocketSession struct {
	conn      *websocket.Conn
	ctx       context.Context
	cancel    context.CancelFunc
	writeMu   sync.Mutex
	mu        sync.Mutex
	pending   map[string]chan wireMessage
	inbox     chan Frame
	closeOnce sync.Once
}

func (s *webSocketSession) Advertise(ctx context.Context, advertisement Advertisement) error {
	response, err := s.request(ctx, wireMessage{Kind: "advertise", Advertisement: &advertisement})
	return wireResponseError(response, err)
}

func (s *webSocketSession) Snapshot(ctx context.Context) (Snapshot, error) {
	response, err := s.request(ctx, wireMessage{Kind: "snapshot"})
	if err := wireResponseError(response, err); err != nil {
		return Snapshot{}, err
	}
	if response.Snapshot == nil {
		return Snapshot{}, errors.New("cluster relay returned no snapshot")
	}
	return *response.Snapshot, nil
}

func (s *webSocketSession) TryAcquireLeadership(ctx context.Context, tunnelID string, ttl time.Duration) (LeaderLease, bool, error) {
	response, err := s.request(ctx, wireMessage{Kind: "leader.acquire", TunnelID: tunnelID, TTLMillis: ttl.Milliseconds()})
	if err := wireResponseError(response, err); err != nil {
		return LeaderLease{}, false, err
	}
	if response.Lease == nil {
		return LeaderLease{}, false, errors.New("cluster relay returned no leader lease")
	}
	return *response.Lease, response.Acquired, nil
}

func (s *webSocketSession) RenewLeadership(ctx context.Context, lease LeaderLease, ttl time.Duration) (LeaderLease, error) {
	response, err := s.request(ctx, wireMessage{Kind: "leader.renew", Lease: &lease, TTLMillis: ttl.Milliseconds()})
	if err := wireResponseError(response, err); err != nil {
		return LeaderLease{}, err
	}
	if response.Lease == nil {
		return LeaderLease{}, errors.New("cluster relay returned no renewed lease")
	}
	return *response.Lease, nil
}

func (s *webSocketSession) ReleaseLeadership(ctx context.Context, lease LeaderLease) error {
	response, err := s.request(ctx, wireMessage{Kind: "leader.release", Lease: &lease})
	return wireResponseError(response, err)
}

func (s *webSocketSession) Leadership(ctx context.Context, tunnelID string) (LeaderLease, bool, error) {
	response, err := s.request(ctx, wireMessage{Kind: "leader.get", TunnelID: tunnelID})
	if err := wireResponseError(response, err); err != nil {
		return LeaderLease{}, false, err
	}
	if !response.Found {
		return LeaderLease{}, false, nil
	}
	if response.Lease == nil {
		return LeaderLease{}, false, errors.New("cluster relay returned invalid leadership response")
	}
	return *response.Lease, true, nil
}

func (s *webSocketSession) Send(ctx context.Context, frame Frame) error {
	response, err := s.request(ctx, wireMessage{Kind: "frame", Frame: &frame})
	return wireResponseError(response, err)
}

func (s *webSocketSession) Receive(ctx context.Context) (Frame, error) {
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

func (s *webSocketSession) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		_ = s.conn.Close(websocket.StatusNormalClosure, "")
		s.failPending(ErrClosed)
	})
	return nil
}

func (s *webSocketSession) request(ctx context.Context, message wireMessage) (wireMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	id, err := rpcID()
	if err != nil {
		return wireMessage{}, err
	}
	message.Version, message.ID = ProtocolVersion, id
	response := make(chan wireMessage, 1)
	s.mu.Lock()
	s.pending[id] = response
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}()
	if err := s.write(ctx, message); err != nil {
		return wireMessage{}, err
	}
	select {
	case <-ctx.Done():
		return wireMessage{}, ctx.Err()
	case <-s.ctx.Done():
		return wireMessage{}, ErrClosed
	case value := <-response:
		return value, nil
	}
}

func (s *webSocketSession) write(ctx context.Context, message wireMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.Write(ctx, websocket.MessageText, data)
}

func (s *webSocketSession) readLoop() {
	for {
		_, data, err := s.conn.Read(s.ctx)
		if err != nil {
			s.cancel()
			s.failPending(err)
			return
		}
		var message wireMessage
		if json.Unmarshal(data, &message) != nil || message.Version != ProtocolVersion {
			continue
		}
		if message.Kind == "frame.event" && message.Frame != nil {
			select {
			case <-s.ctx.Done():
				return
			case s.inbox <- *message.Frame:
			}
			continue
		}
		s.mu.Lock()
		pending := s.pending[message.ID]
		s.mu.Unlock()
		if pending != nil {
			select {
			case pending <- message:
			default:
			}
		}
	}
}

func (s *webSocketSession) failPending(err error) {
	if err == nil {
		err = ErrClosed
	}
	s.mu.Lock()
	values := make([]chan wireMessage, 0, len(s.pending))
	for id, pending := range s.pending {
		values = append(values, pending)
		delete(s.pending, id)
	}
	s.mu.Unlock()
	for _, pending := range values {
		select {
		case pending <- wireMessage{Version: ProtocolVersion, Kind: "response", Error: err.Error()}:
		default:
		}
	}
}

func wireResponseError(response wireMessage, err error) error {
	if err != nil {
		return err
	}
	if response.Error == "" {
		return nil
	}
	if response.Error == ErrLeaseLost.Error() {
		return ErrLeaseLost
	}
	if response.Error == ErrClosed.Error() {
		return ErrClosed
	}
	return errors.New(response.Error)
}
