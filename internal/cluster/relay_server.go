package cluster

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const (
	defaultRelayMaxConnections       = 256
	defaultRelayMaxRequestsPerSecond = 256
	defaultRelayHelloTimeout         = 10 * time.Second
	defaultRelayIdleTimeout          = 30 * time.Second
	defaultRelayWriteTimeout         = 10 * time.Second
)

type RelayServerOptions struct {
	MaxConnections       int
	MaxRequestsPerSecond int
	HelloTimeout         time.Duration
	IdleTimeout          time.Duration
	WriteTimeout         time.Duration
}

type RelayHealth struct {
	OK                bool      `json:"ok"`
	StartedAt         time.Time `json:"started_at"`
	UptimeSeconds     int64     `json:"uptime_seconds"`
	ActiveConnections int64     `json:"active_connections"`
}

type RelayMetrics struct {
	StartedAt           time.Time `json:"started_at"`
	ActiveConnections   int64     `json:"active_connections"`
	AcceptedConnections uint64    `json:"accepted_connections"`
	RejectedConnections uint64    `json:"rejected_connections"`
	RequestsTotal       uint64    `json:"requests_total"`
	FramesTotal         uint64    `json:"frames_total"`
	ErrorsTotal         uint64    `json:"errors_total"`
	MemberCount         int       `json:"member_count"`
	OnlineMemberCount   int       `json:"online_member_count"`
	WorkspaceCount      int       `json:"workspace_count"`
	LeaderCount         int       `json:"leader_count"`
	CatalogCompatible   bool      `json:"catalog_compatible"`
	CatalogError        string    `json:"catalog_error,omitempty"`
}

type RelayServer struct {
	Token    string
	Relay    *MemoryRelay
	Backend  RelayBackend
	options  RelayServerOptions
	started  time.Time
	closed   atomic.Bool
	active   atomic.Int64
	accepted atomic.Uint64
	rejected atomic.Uint64
	requests atomic.Uint64
	frames   atomic.Uint64
	errors   atomic.Uint64
	connsMu  sync.Mutex
	conns    map[*websocket.Conn]struct{}
}

func DefaultRelayServerOptions() RelayServerOptions {
	return RelayServerOptions{MaxConnections: defaultRelayMaxConnections, MaxRequestsPerSecond: defaultRelayMaxRequestsPerSecond, HelloTimeout: defaultRelayHelloTimeout, IdleTimeout: defaultRelayIdleTimeout, WriteTimeout: defaultRelayWriteTimeout}
}

func NewRelayServer(token string) *RelayServer {
	relay := NewMemoryRelay()
	return NewRelayServerWithBackend(token, relay, RelayServerOptions{})
}

func NewRelayServerWithBackend(token string, backend RelayBackend, options RelayServerOptions) *RelayServer {
	options = normalizeRelayServerOptions(options)
	server := &RelayServer{Token: token, Backend: backend, options: options, started: time.Now().UTC(), conns: map[*websocket.Conn]struct{}{}}
	if relay, ok := backend.(*MemoryRelay); ok {
		server.Relay = relay
	}
	return server
}

func normalizeRelayServerOptions(options RelayServerOptions) RelayServerOptions {
	defaults := DefaultRelayServerOptions()
	if options.MaxConnections <= 0 {
		options.MaxConnections = defaults.MaxConnections
	}
	if options.MaxRequestsPerSecond <= 0 {
		options.MaxRequestsPerSecond = defaults.MaxRequestsPerSecond
	}
	if options.HelloTimeout <= 0 {
		options.HelloTimeout = defaults.HelloTimeout
	}
	if options.IdleTimeout <= 0 {
		options.IdleTimeout = defaults.IdleTimeout
	}
	if options.WriteTimeout <= 0 {
		options.WriteTimeout = defaults.WriteTimeout
	}
	return options
}

func (s *RelayServer) Handler(path string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(path, s)
	mux.HandleFunc("/health", s.HealthHandler())
	mux.HandleFunc("/metrics", s.MetricsHandler())
	return mux
}

func (s *RelayServer) Health() RelayHealth {
	if s == nil {
		return RelayHealth{}
	}
	started := s.started
	if started.IsZero() {
		started = time.Now().UTC()
	}
	return RelayHealth{OK: s.backend() != nil && !s.closed.Load(), StartedAt: started, UptimeSeconds: int64(time.Since(started).Seconds()), ActiveConnections: s.active.Load()}
}

func (s *RelayServer) Metrics() RelayMetrics {
	if s == nil {
		return RelayMetrics{}
	}
	health := s.Health()
	metrics := RelayMetrics{StartedAt: health.StartedAt, ActiveConnections: health.ActiveConnections, AcceptedConnections: s.accepted.Load(), RejectedConnections: s.rejected.Load(), RequestsTotal: s.requests.Load(), FramesTotal: s.frames.Load(), ErrorsTotal: s.errors.Load(), CatalogCompatible: true}
	backend := s.backend()
	if backend == nil {
		return metrics
	}
	snapshot := backend.Snapshot()
	metrics.MemberCount = len(snapshot.Members)
	for _, member := range snapshot.Members {
		if member.Online {
			metrics.OnlineMemberCount++
		}
	}
	metrics.WorkspaceCount = len(snapshot.Workspaces)
	metrics.LeaderCount = len(snapshot.Leaders)
	metrics.CatalogCompatible = snapshot.CatalogCompatible
	metrics.CatalogError = snapshot.CatalogError
	return metrics
}

func (s *RelayServer) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeRelayJSON(w, s.Health())
	}
}

func (s *RelayServer) MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !bearerEqual(r.Header.Get("Authorization"), s.Token) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeRelayJSON(w, s.Metrics())
	}
}

func (s *RelayServer) Close() error {
	if s == nil || !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.connsMu.Lock()
	connections := make([]*websocket.Conn, 0, len(s.conns))
	for conn := range s.conns {
		connections = append(connections, conn)
	}
	s.connsMu.Unlock()
	for _, conn := range connections {
		_ = conn.CloseNow()
	}
	return nil
}

func (s *RelayServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backend := s.backend()
	if backend == nil || s.closed.Load() {
		http.Error(w, "cluster relay unavailable", http.StatusServiceUnavailable)
		return
	}
	if !bearerEqual(r.Header.Get("Authorization"), s.Token) {
		s.rejected.Add(1)
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !s.acquireConnection() {
		s.rejected.Add(1)
		http.Error(w, "cluster relay connection limit reached", http.StatusServiceUnavailable)
		return
	}
	defer s.active.Add(-1)
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{clusterSubprotocol}})
	if err != nil {
		s.errors.Add(1)
		return
	}
	s.accepted.Add(1)
	s.trackConnection(conn, true)
	defer s.trackConnection(conn, false)
	defer conn.Close(websocket.StatusNormalClosure, "")
	if conn.Subprotocol() != clusterSubprotocol {
		s.errors.Add(1)
		_ = conn.Close(websocket.StatusPolicyViolation, "cluster subprotocol required")
		return
	}
	conn.SetReadLimit(maxWireMessageSize)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	_, data, err := relayRead(ctx, conn, s.options.HelloTimeout)
	if err != nil {
		s.errors.Add(1)
		return
	}
	var hello wireMessage
	if json.Unmarshal(data, &hello) != nil || hello.Version != ProtocolVersion || hello.Kind != "hello" || hello.Advertisement == nil {
		s.errors.Add(1)
		_ = s.writeWire(ctx, conn, &sync.Mutex{}, wireMessage{Version: ProtocolVersion, Kind: "response", ID: hello.ID, Error: "cluster hello is required"})
		return
	}
	session, err := backend.Connect(ctx, *hello.Advertisement)
	if err != nil {
		s.errors.Add(1)
		_ = s.writeWire(ctx, conn, &sync.Mutex{}, wireMessage{Version: ProtocolVersion, Kind: "response", ID: hello.ID, Error: err.Error()})
		return
	}
	defer session.Close()
	writeMu := &sync.Mutex{}
	if err := s.writeWire(ctx, conn, writeMu, wireMessage{Version: ProtocolVersion, Kind: "response", ID: hello.ID}); err != nil {
		s.errors.Add(1)
		return
	}
	go s.forwardFrames(ctx, session, conn, writeMu)
	limiter := relayRateLimiter{limit: s.options.MaxRequestsPerSecond}
	for {
		_, data, err := relayRead(ctx, conn, s.options.IdleTimeout)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				s.errors.Add(1)
			}
			return
		}
		if !limiter.Allow(time.Now()) {
			s.rejected.Add(1)
			s.errors.Add(1)
			_ = conn.Close(websocket.StatusPolicyViolation, "cluster relay rate limit exceeded")
			return
		}
		var request wireMessage
		if json.Unmarshal(data, &request) != nil || request.Version != ProtocolVersion || request.ID == "" {
			s.errors.Add(1)
			continue
		}
		s.requests.Add(1)
		if request.Kind == "frame" {
			s.frames.Add(1)
		}
		response := s.handleWireRequest(ctx, session, request)
		response.Version, response.Kind, response.ID = ProtocolVersion, "response", request.ID
		if response.Error != "" {
			s.errors.Add(1)
		}
		if err := s.writeWire(ctx, conn, writeMu, response); err != nil {
			s.errors.Add(1)
			return
		}
	}
}

func (s *RelayServer) backend() RelayBackend {
	if s == nil {
		return nil
	}
	if s.Backend != nil {
		return s.Backend
	}
	return s.Relay
}

func (s *RelayServer) acquireConnection() bool {
	limit := int64(s.options.MaxConnections)
	for {
		current := s.active.Load()
		if current >= limit {
			return false
		}
		if s.active.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (s *RelayServer) trackConnection(conn *websocket.Conn, add bool) {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	if add {
		s.conns[conn] = struct{}{}
		return
	}
	delete(s.conns, conn)
}

func (s *RelayServer) forwardFrames(ctx context.Context, session Session, conn *websocket.Conn, writeMu *sync.Mutex) {
	for {
		frame, err := session.Receive(ctx)
		if err != nil {
			return
		}
		if err := s.writeWire(ctx, conn, writeMu, wireMessage{Version: ProtocolVersion, Kind: "frame.event", Frame: &frame}); err != nil {
			s.errors.Add(1)
			_ = conn.CloseNow()
			return
		}
	}
}

func (s *RelayServer) handleWireRequest(ctx context.Context, session Session, request wireMessage) wireMessage {
	response := wireMessage{}
	var err error
	switch request.Kind {
	case "advertise":
		if request.Advertisement == nil {
			err = errors.New("cluster advertisement is required")
		} else {
			err = session.Advertise(ctx, *request.Advertisement)
		}
	case "snapshot":
		var snapshot Snapshot
		snapshot, err = session.Snapshot(ctx)
		response.Snapshot = &snapshot
	case "leader.acquire":
		var lease LeaderLease
		lease, response.Acquired, err = session.TryAcquireLeadership(ctx, request.TunnelID, time.Duration(request.TTLMillis)*time.Millisecond)
		response.Lease = &lease
	case "leader.renew":
		if request.Lease == nil {
			err = errors.New("cluster leader lease is required")
		} else {
			var lease LeaderLease
			lease, err = session.RenewLeadership(ctx, *request.Lease, time.Duration(request.TTLMillis)*time.Millisecond)
			response.Lease = &lease
		}
	case "leader.release":
		if request.Lease == nil {
			err = errors.New("cluster leader lease is required")
		} else {
			err = session.ReleaseLeadership(ctx, *request.Lease)
		}
	case "leader.get":
		var lease LeaderLease
		lease, response.Found, err = session.Leadership(ctx, request.TunnelID)
		if response.Found {
			response.Lease = &lease
		}
	case "frame":
		if request.Frame == nil {
			err = errors.New("cluster frame is required")
		} else {
			err = session.Send(ctx, *request.Frame)
		}
	default:
		err = fmt.Errorf("unsupported cluster relay request: %s", request.Kind)
	}
	if err != nil {
		response.Error = err.Error()
	}
	return response
}

func (s *RelayServer) writeWire(ctx context.Context, conn *websocket.Conn, mu *sync.Mutex, message wireMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, s.options.WriteTimeout)
	defer cancel()
	mu.Lock()
	defer mu.Unlock()
	return conn.Write(writeCtx, websocket.MessageText, data)
}

func relayRead(ctx context.Context, conn *websocket.Conn, timeout time.Duration) (websocket.MessageType, []byte, error) {
	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return conn.Read(readCtx)
}

type relayRateLimiter struct {
	limit       int
	windowStart time.Time
	count       int
}

func (l *relayRateLimiter) Allow(now time.Time) bool {
	if l.limit <= 0 {
		return true
	}
	if l.windowStart.IsZero() || now.Sub(l.windowStart) >= time.Second {
		l.windowStart = now
		l.count = 0
	}
	l.count++
	return l.count <= l.limit
}

func writeRelayJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func bearerEqual(header, token string) bool {
	prefix, value, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(prefix, "Bearer") || token == "" || len(value) != len(token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(value), []byte(token)) == 1
}
