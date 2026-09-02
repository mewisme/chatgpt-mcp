package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/cluster"
)

func TestValidateClusterRelayListen(t *testing.T) {
	for _, test := range []struct {
		name          string
		value         string
		allowInsecure bool
		want          string
		wantErr       bool
	}{
		{name: "IPv4 loopback", value: "127.0.0.1:37423", want: "127.0.0.1:37423"},
		{name: "IPv6 loopback", value: "[::1]:37423", want: "[::1]:37423"},
		{name: "localhost", value: "localhost:37423", want: "localhost:37423"},
		{name: "remote denied", value: "0.0.0.0:37423", wantErr: true},
		{name: "remote explicit", value: "0.0.0.0:37423", allowInsecure: true, want: "0.0.0.0:37423"},
		{name: "missing host", value: ":37423", wantErr: true},
		{name: "bad port", value: "127.0.0.1:70000", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := validateClusterRelayListen(test.value, test.allowInsecure)
			if test.wantErr {
				if err == nil {
					t.Fatalf("validateClusterRelayListen(%q) = %q, want error", test.value, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("validateClusterRelayListen(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestNormalizeClusterRelayPath(t *testing.T) {
	for _, test := range []struct {
		value   string
		want    string
		wantErr bool
	}{
		{value: "/cluster", want: "/cluster"},
		{value: "/cluster/", want: "/cluster"},
		{value: "/", want: "/"},
		{value: "cluster", wantErr: true},
		{value: "/cluster?token=x", wantErr: true},
		{value: "/health", wantErr: true},
		{value: "/metrics/", wantErr: true},
	} {
		got, err := normalizeClusterRelayPath(test.value)
		if test.wantErr {
			if err == nil {
				t.Fatalf("normalizeClusterRelayPath(%q) = %q, want error", test.value, got)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("normalizeClusterRelayPath(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestValidateClusterRelayOptions(t *testing.T) {
	valid := cluster.DefaultRelayServerOptions()
	if err := validateClusterRelayOptions(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*cluster.RelayServerOptions){
		func(value *cluster.RelayServerOptions) { value.MaxConnections = 0 },
		func(value *cluster.RelayServerOptions) { value.MaxRequestsPerSecond = 0 },
		func(value *cluster.RelayServerOptions) { value.HelloTimeout = 0 },
		func(value *cluster.RelayServerOptions) { value.IdleTimeout = -time.Second },
		func(value *cluster.RelayServerOptions) { value.WriteTimeout = 0 },
	} {
		value := valid
		mutate(&value)
		if err := validateClusterRelayOptions(value); err == nil {
			t.Fatalf("expected invalid relay options: %#v", value)
		}
	}
}

func TestClusterRelayTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.token")
	if err := os.WriteFile(path, []byte(" file-secret \n"), 0600); err != nil {
		t.Fatal(err)
	}
	token, err := clusterRelayToken(path)
	if err != nil || token != "file-secret" {
		t.Fatalf("token = %q err=%v", token, err)
	}
	if err := os.WriteFile(path, []byte("\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := clusterRelayToken(path); err == nil {
		t.Fatal("expected empty token file to fail")
	}
}

func TestClusterRelayCommandHierarchy(t *testing.T) {
	cmd := clusterCommand()
	resolved, _, err := cmd.Find([]string{"relay"})
	if err != nil || resolved.Name() != "relay" {
		t.Fatalf("cluster relay resolved to %v: %v", resolved, err)
	}
}
