package socks

import (
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func tcpPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
	t.Helper()
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	a, err := net.DialTCP("tcp", nil, listener.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	b, err := listener.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	for _, c := range []*net.TCPConn{a, b} {
		if err := c.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	return a, b
}

func TestProxyConnectionDrainsHalfClosedResponse(t *testing.T) {
	client, incoming := tcpPair(t)
	outgoing, server := tcpPair(t)
	done := make(chan error, 1)
	go func() { done <- proxyConnection(incoming, incoming, outgoing, time.Second) }()
	if _, err := io.WriteString(client, "request"); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	request, err := io.ReadAll(server)
	if err != nil || string(request) != "request" {
		t.Fatalf("request: %q, %v", request, err)
	}
	if _, err := io.WriteString(server, "response after EOF"); err != nil {
		t.Fatal(err)
	}
	if err := server.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	response, err := io.ReadAll(client)
	if err != nil || string(response) != "response after EOF" {
		t.Fatalf("response: %q, %v", response, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not finish")
	}
}

func TestProxyConnectionBoundsHalfCloseDrain(t *testing.T) {
	for _, clientEOF := range []bool{false, true} {
		t.Run(map[bool]string{false: "server EOF", true: "client EOF"}[clientEOF], func(t *testing.T) {
			client, incoming := tcpPair(t)
			outgoing, server := tcpPair(t)
			done := make(chan error, 1)
			go func() { done <- proxyConnection(incoming, incoming, outgoing, 20*time.Millisecond) }()
			peer := server
			if clientEOF {
				peer = client
			}
			if err := peer.CloseWrite(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), "drain timed out") {
					t.Fatalf("unexpected result: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("proxy did not time out")
			}
			for _, peer := range []*net.TCPConn{client, server} {
				if _, err := io.ReadAll(peer); err != nil {
					t.Fatalf("peer not closed: %v", err)
				}
			}
		})
	}
}
