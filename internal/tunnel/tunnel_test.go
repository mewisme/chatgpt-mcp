package tunnel

import (
	"os"
	"testing"
	"time"
)

func TestDisabledTunnelDoesNotStart(t *testing.T) {
	client := NewConfigured(Config{Enabled: false})
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	if client.Status().Running {
		t.Fatal("disabled tunnel must not run")
	}
}

func TestEnabledTunnelRequiresCommand(t *testing.T) {
	client := NewConfigured(Config{Enabled: true})
	if err := client.Start(); err == nil {
		t.Fatal("expected missing command error")
	}
}

func TestTunnelProcessLifecycle(t *testing.T) {
	if os.Getenv("CHATGPT_MCP_TUNNEL_TEST_HELPER") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	t.Setenv("CHATGPT_MCP_TUNNEL_TEST_HELPER", "1")
	client := NewConfigured(Config{Enabled: true, ID: "test-id", APIKey: "secret", Command: os.Args[0], Args: []string{"-test.run=TestTunnelProcessLifecycle"}, Origin: "http://127.0.0.1:3000"})
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	status := client.Status()
	if !status.Running || status.PID == 0 {
		t.Fatalf("expected running tunnel, got %+v", status)
	}
	if err := client.Stop(); err != nil {
		t.Fatal(err)
	}
	if client.Status().Running {
		t.Fatal("expected stopped tunnel")
	}
	env := client.Environment()
	if env["CHATGPT_MCP_TUNNEL_ID"] != "test-id" || env["CHATGPT_MCP_TUNNEL_API_KEY"] != "secret" || env["CHATGPT_MCP_TUNNEL_ORIGIN"] != "http://127.0.0.1:3000" {
		t.Fatalf("unexpected tunnel environment: %+v", env)
	}
}
