package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRelayServerHealthAndMetrics(t *testing.T) {
	relay := NewRelayServer("secret")
	server := httptest.NewServer(relay.Handler("/cluster"))
	defer server.Close()

	response, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var health RelayHealth
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&health) != nil || !health.OK || health.ActiveConnections != 0 {
		t.Fatalf("health = %#v status=%d", health, response.StatusCode)
	}

	response, err = http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated metrics status = %d", response.StatusCode)
	}

	transport := NewWebSocketTransport("ws"+strings.TrimPrefix(server.URL, "http")+"/cluster", "secret")
	session, err := transport.Connect(context.Background(), Advertisement{InstanceID: "inst_a", Name: "a", CatalogHash: "cat", Workspaces: []string{"ws_a"}})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}

	request, err := http.NewRequest(http.MethodGet, server.URL+"/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer secret")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var metrics RelayMetrics
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&metrics) != nil {
		t.Fatalf("metrics status = %d", response.StatusCode)
	}
	if metrics.ActiveConnections != 1 || metrics.AcceptedConnections != 1 || metrics.RequestsTotal < 1 || metrics.MemberCount != 1 || metrics.OnlineMemberCount != 1 || metrics.WorkspaceCount != 1 || !metrics.CatalogCompatible {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestRelayServerConnectionLimit(t *testing.T) {
	options := DefaultRelayServerOptions()
	options.MaxConnections = 1
	relay := NewRelayServerWithBackend("secret", NewMemoryRelay(), options)
	server := httptest.NewServer(relay.Handler("/cluster"))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/cluster"
	first, err := NewWebSocketTransport(url, "secret").Connect(context.Background(), Advertisement{InstanceID: "inst_a", Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := NewWebSocketTransport(url, "secret").Connect(context.Background(), Advertisement{InstanceID: "inst_b", Name: "b"}); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("second connection error = %v", err)
	}
	if metrics := relay.Metrics(); metrics.ActiveConnections != 1 || metrics.RejectedConnections != 1 {
		t.Fatalf("connection-limit metrics = %#v", metrics)
	}
}

func TestRelayServerCloseDropsWebSocketSessions(t *testing.T) {
	relay := NewRelayServer("secret")
	server := httptest.NewServer(relay.Handler("/cluster"))
	defer server.Close()
	session, err := NewWebSocketTransport("ws"+strings.TrimPrefix(server.URL, "http")+"/cluster", "secret").Connect(context.Background(), Advertisement{InstanceID: "inst_a", Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_, err = session.Snapshot(ctx)
		cancel()
		if errors.Is(err, ErrClosed) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session remained usable after relay close: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if health := relay.Health(); health.OK {
		t.Fatalf("closed relay health = %#v", health)
	}
}

func TestRelayRateLimiter(t *testing.T) {
	limiter := relayRateLimiter{limit: 2}
	now := time.Now()
	if !limiter.Allow(now) || !limiter.Allow(now) || limiter.Allow(now) {
		t.Fatal("rate limiter did not enforce fixed-window limit")
	}
	if !limiter.Allow(now.Add(time.Second)) {
		t.Fatal("rate limiter did not reset after one second")
	}
}
