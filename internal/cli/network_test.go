package cli

import (
	"net"
	"reflect"
	"testing"
)

func TestServerBindHost(t *testing.T) {
	if got := serverBindHost(false); got != "127.0.0.1" {
		t.Fatalf("loopback bind = %q", got)
	}
	if got := serverBindHost(true); got != "0.0.0.0" {
		t.Fatalf("exposed bind = %q", got)
	}
}

func TestNormalizeNetworkAddresses(t *testing.T) {
	candidates := []networkCandidate{
		{IP: net.ParseIP("127.0.0.1"), Interface: "loopback", Flags: net.FlagUp | net.FlagLoopback},
		{IP: net.ParseIP("192.168.1.20"), Interface: "Wi-Fi", Flags: net.FlagUp},
		{IP: net.ParseIP("10.0.0.4"), Interface: "Ethernet", Flags: net.FlagUp},
		{IP: net.ParseIP("192.168.1.20"), Interface: "duplicate", Flags: net.FlagUp},
		{IP: net.ParseIP("203.0.113.9"), Interface: "Public", Flags: net.FlagUp},
		{IP: net.ParseIP("172.20.0.2"), Interface: "Down", Flags: 0},
		{IP: net.ParseIP("2001:db8::1"), Interface: "IPv6", Flags: net.FlagUp},
	}
	got := normalizeNetworkAddresses(candidates)
	want := []runtimeAddress{
		{Host: "10.0.0.4", Interface: "Ethernet", Scope: "lan"},
		{Host: "192.168.1.20", Interface: "Wi-Fi", Scope: "lan"},
		{Host: "203.0.113.9", Interface: "Public", Scope: "network"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("addresses = %#v, want %#v", got, want)
	}
}

func TestRuntimeAddressesAlwaysIncludeLoopback(t *testing.T) {
	got := runtimeAddresses(false)
	want := []runtimeAddress{{Host: "127.0.0.1", Scope: "local"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("addresses = %#v, want %#v", got, want)
	}
}

func TestEndpointURL(t *testing.T) {
	if got := endpointURL("127.0.0.1", 37421, "/mcp"); got != "http://127.0.0.1:37421/mcp" {
		t.Fatalf("url = %q", got)
	}
}
