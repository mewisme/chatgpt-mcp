package tunnel

import (
	"context"
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
	status = client.Status()
	if status.Running || status.PID != 0 {
		t.Fatalf("expected stopped and reaped tunnel, got %+v", status)
	}
	if status.LastError != "" {
		t.Fatalf("expected intentional stop not to set last error, got %q", status.LastError)
	}
	env := client.Environment()
	if env["CHATGPT_MCP_TUNNEL_ID"] != "test-id" || env["CHATGPT_MCP_TUNNEL_API_KEY"] != redactedSecret || env["CHATGPT_MCP_TUNNEL_ORIGIN"] != "http://127.0.0.1:3000" {
		t.Fatalf("unexpected redacted tunnel environment: %+v", env)
	}
}

func TestTunnelContextCancellationStopsAndReapsProcess(t *testing.T) {
	if os.Getenv("CHATGPT_MCP_TUNNEL_CANCEL_HELPER") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	t.Setenv("CHATGPT_MCP_TUNNEL_CANCEL_HELPER", "1")
	ctx, cancel := context.WithCancel(context.Background())
	client := NewConfigured(Config{Enabled: true, Command: os.Args[0], Args: []string{"-test.run=TestTunnelContextCancellationStopsAndReapsProcess"}})
	if err := client.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	if !client.Status().Running {
		t.Fatal("expected running tunnel before context cancellation")
	}
	cancel()

	deadline := time.Now().Add(defaultStopTimeout + time.Second)
	for client.Status().Running {
		if time.Now().After(deadline) {
			t.Fatalf("tunnel remained running after context cancellation: %+v", client.Status())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status := client.Status(); status.PID != 0 || status.LastError != "" {
		t.Fatalf("unexpected stopped tunnel status: %+v", status)
	}
}

func TestTunnelEnvironmentDoesNotExposeAPIKey(t *testing.T) {
	client := NewConfigured(Config{Enabled: true, ID: "id", APIKey: "super-secret"})
	env := client.Environment()
	if env["CHATGPT_MCP_TUNNEL_API_KEY"] != redactedSecret {
		t.Fatalf("API key was exposed: %+v", env)
	}
	if client.Config().APIKey != "super-secret" {
		t.Fatal("runtime config lost the API key")
	}
}
