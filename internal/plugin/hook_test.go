package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHookManifestRequiresDeclaredPermissions(t *testing.T) {
	for _, test := range []struct {
		name        string
		capability  Capability
		permissions []Permission
		want        string
	}{
		{name: "pre missing control", capability: CapabilityHookPreToolUse, permissions: []Permission{PermissionProcessExecute}, want: string(PermissionHookToolControl)},
		{name: "pre missing execute", capability: CapabilityHookPreToolUse, permissions: []Permission{PermissionHookToolControl}, want: string(PermissionProcessExecute)},
		{name: "post missing observe", capability: CapabilityHookPostToolUse, permissions: []Permission{PermissionProcessExecute}, want: string(PermissionHookToolObserve)},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest := testManifest("hook-test", "1.0.0", test.capability)
			manifest.Type = "hook"
			manifest.Permissions = test.permissions
			if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
	manifest := testManifest("hook-ok", "1.0.0", CapabilityHookPreToolUse)
	manifest.Type = "hook"
	manifest.Permissions = []Permission{PermissionProcessExecute, PermissionHookToolControl}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestHookDispatcherPreToolUseIsDeterministicAndFailClosed(t *testing.T) {
	store := testStore(t)
	installHookTestPlugin(t, store, "a-hook", CapabilityHookPreToolUse, PermissionHookToolControl)
	installHookTestPlugin(t, store, "b-hook", CapabilityHookPreToolUse, PermissionHookToolControl)
	called := []PluginID{}
	dispatcher := NewHookDispatcherWithRunner(store, func(_ context.Context, provider CapabilityProvider, _ HookEvent) (HookResult, error) {
		called = append(called, provider.PluginID)
		if provider.PluginID == "b-hook" {
			return HookResult{Schema: HookSchema, Decision: HookDecisionRequireApproval, Reason: "review required"}, nil
		}
		return HookResult{Schema: HookSchema, Decision: HookDecisionContinue}, nil
	})
	result, err := dispatcher.PreToolUse(context.Background(), HookEvent{Provenance: HookProvenance{ExecutionID: "call_1", Origin: HookOriginAgent}, Tool: "run_command"})
	if err != nil {
		t.Fatal(err)
	}
	if len(called) != 2 || called[0] != "a-hook" || called[1] != "b-hook" || result.Decision != HookDecisionRequireApproval {
		t.Fatalf("called=%#v result=%#v", called, result)
	}
	if len(result.Providers) != 2 || result.Providers[0].PluginID != "a-hook" || result.Providers[1].PluginID != "b-hook" || result.Providers[1].Version != "1.0.0" || result.Providers[1].Capability != CapabilityHookPreToolUse {
		t.Fatalf("hook provider metadata = %#v", result.Providers)
	}

	dispatcher.runner = func(_ context.Context, _ CapabilityProvider, _ HookEvent) (HookResult, error) {
		return HookResult{}, errors.New("boom")
	}
	if _, err := dispatcher.PreToolUse(context.Background(), HookEvent{Provenance: HookProvenance{ExecutionID: "call_2", Origin: HookOriginAgent}, Tool: "run_command"}); err == nil {
		t.Fatal("failing pre hook did not fail closed")
	}
}

func TestHookDispatcherPreToolUseTimeoutFailsClosed(t *testing.T) {
	store := testStore(t)
	installHookTestPlugin(t, store, "timeout-hook", CapabilityHookPreToolUse, PermissionHookToolControl)
	dispatcher := NewHookDispatcherWithRunner(store, func(ctx context.Context, _ CapabilityProvider, _ HookEvent) (HookResult, error) {
		<-ctx.Done()
		return HookResult{}, ctx.Err()
	})
	dispatcher.preTimeout = 10 * time.Millisecond
	_, err := dispatcher.PreToolUse(context.Background(), HookEvent{Provenance: HookProvenance{ExecutionID: "call_timeout", Origin: HookOriginAgent}, Tool: "run_command"})
	if !errors.Is(err, context.DeadlineExceeded) && (err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error())) {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestHookDispatcherObservationQueueIsBoundedAndNonRecursive(t *testing.T) {
	store := testStore(t)
	installHookTestPlugin(t, store, "observe-hook", CapabilityHookPostToolUse, PermissionHookToolObserve)
	blocked := make(chan struct{})
	started := make(chan struct{}, 1)
	var mu sync.Mutex
	count := 0
	dispatcher := NewHookDispatcherWithRunner(store, func(ctx context.Context, _ CapabilityProvider, _ HookEvent) (HookResult, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-blocked:
		case <-ctx.Done():
			return HookResult{}, ctx.Err()
		}
		mu.Lock()
		count++
		mu.Unlock()
		return HookResult{Schema: HookSchema, Decision: HookDecisionContinue}, nil
	})
	dispatcher.observationTimeout = time.Second
	event := HookEvent{Type: HookEventPostToolUse, Provenance: HookProvenance{ExecutionID: "call_1", Origin: HookOriginAgent}, Tool: "run_command"}
	if !dispatcher.Observe(event) {
		t.Fatal("first observation was not queued")
	}
	<-started
	for index := 0; index < cap(dispatcher.queue)+16; index++ {
		dispatcher.Observe(event)
	}
	if dispatcher.Stats().Dropped == 0 {
		t.Fatal("bounded observation queue did not report drops")
	}
	if dispatcher.Observe(HookEvent{Type: HookEventPostToolUse, Provenance: HookProvenance{ExecutionID: "call_hook", Origin: HookOriginHook, HookDepth: 1}, Tool: "run_command"}) {
		t.Fatal("hook-originated observation was recursively queued")
	}
	close(blocked)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := dispatcher.WaitIdle(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	observed := count
	mu.Unlock()
	if observed == 0 || observed > defaultObservationQueueSize+1 {
		t.Fatalf("observation count = %d", observed)
	}
}

func TestHookProvenanceContextRoundTrip(t *testing.T) {
	want := HookProvenance{ExecutionID: "call_child", ParentExecutionID: "call_parent", Origin: HookOriginHook, HookDepth: 1}
	ctx := WithHookProvenance(context.Background(), want)
	got, ok := HookProvenanceFromContext(ctx)
	if !ok || got != want {
		t.Fatalf("provenance = %#v ok=%t", got, ok)
	}
}

func TestInstalledHookExecutableUsesStrictSubprocessProtocol(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX hook protocol fixture")
	}
	store := testStore(t)
	payload := t.TempDir()
	if err := os.MkdirAll(filepath.Join(payload, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat >/dev/null\nif [ -n \"${CGM_HOOK_SECRET+x}\" ] || [ \"$CHATGPT_MCP_TOOL_CONTEXT\" != 1 ]; then printf '%s' '{\"schema\":1,\"decision\":\"deny\",\"reason\":\"environment boundary failed\"}'; else printf '%s' '{\"schema\":1,\"decision\":\"continue\"}'; fi\n"
	if err := os.WriteFile(filepath.Join(payload, "bin", "hook-exec"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	manifest := testManifest("hook-exec", "1.0.0", CapabilityHookPreToolUse)
	manifest.Type = "hook"
	manifest.Permissions = []Permission{PermissionProcessExecute, PermissionHookToolControl}
	platform := runtime.GOOS + "/" + runtime.GOARCH
	if platform != "linux/amd64" {
		delete(manifest.Platforms, "linux/amd64")
	}
	manifest.Platforms[platform] = PlatformArtifact{Artifact: "hook-exec.tar.gz", SHA256: strings.Repeat("a", 64), Archive: "tar.gz", Entrypoint: "bin/hook-exec"}
	store.runtime.OS, store.runtime.Arch = runtime.GOOS, runtime.GOARCH
	if _, err := store.Install(manifest, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("hook-exec", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CGM_HOOK_SECRET", "must-not-leak")
	dispatcher := NewHookDispatcher(store)
	result, err := dispatcher.PreToolUse(context.Background(), HookEvent{Provenance: HookProvenance{ExecutionID: "call_1", Origin: HookOriginAgent}, Tool: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != HookDecisionContinue {
		t.Fatalf("hook result = %#v", result)
	}
}

func TestSafeHookEnvironmentOmitsAuthTokens(t *testing.T) {
	t.Setenv("CHATGPT_MCP_MCP_TOKEN", "mcp_"+strings.Repeat("a", 32))
	t.Setenv("MCP_TOKEN", "mcp_"+strings.Repeat("b", 32))
	t.Setenv("ADMIN_TOKEN", "admin_"+strings.Repeat("c", 32))
	t.Setenv("HOME", t.TempDir())
	joined := strings.Join(safeHookEnvironment(), "\n")
	for _, leaked := range []string{"mcp_", "MCP_TOKEN", "ADMIN_TOKEN", "CHATGPT_MCP_MCP_TOKEN"} {
		if strings.Contains(joined, leaked) {
			t.Fatalf("hook env leaked %q: %s", leaked, joined)
		}
	}
}

func installHookTestPlugin(t *testing.T, store *Store, id string, capability Capability, permission Permission, scopes ...PluginScope) {
	t.Helper()
	manifest := testManifest(id, "1.0.0", capability)
	if len(scopes) > 0 {
		manifest = testScopedManifest(id, "1.0.0", capability, scopes...)
	}
	manifest.Type = "hook"
	manifest.Permissions = []Permission{PermissionProcessExecute, permission}
	if _, err := store.Install(manifest, testPayload(t, id)); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(PluginID(id), "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
}

func TestHookDispatcherUsesWorkspacePluginsOnlyWithWorkspaceID(t *testing.T) {
	global := testStore(t)
	workspace := testWorkspaceStore(t)
	installHookTestPlugin(t, global, "global-hook", CapabilityHookPreToolUse, PermissionHookToolControl)
	installHookTestPlugin(t, workspace, "workspace-hook", CapabilityHookPreToolUse, PermissionHookToolControl, ScopeWorkspace)
	called := []PluginID{}
	dispatcher := NewHookDispatcherWithRunner(global, func(_ context.Context, provider CapabilityProvider, _ HookEvent) (HookResult, error) {
		called = append(called, provider.PluginID)
		return HookResult{Schema: HookSchema, Decision: HookDecisionContinue}, nil
	})
	dispatcher.SetWorkspaceStore(func(id string) *Store {
		if id == "ws_demo" {
			return workspace
		}
		return nil
	})
	if _, err := dispatcher.PreToolUse(context.Background(), HookEvent{Provenance: HookProvenance{ExecutionID: "call_1", Origin: HookOriginAgent}, Tool: "run_command"}); err != nil {
		t.Fatal(err)
	}
	if len(called) != 1 || called[0] != "global-hook" {
		t.Fatalf("global context called=%#v", called)
	}
	called = nil
	if _, err := dispatcher.PreToolUse(context.Background(), HookEvent{Provenance: HookProvenance{ExecutionID: "call_2", Origin: HookOriginAgent}, Tool: "run_command", WorkspaceID: "ws_demo"}); err != nil {
		t.Fatal(err)
	}
	if len(called) != 2 || called[0] != "global-hook" || called[1] != "workspace-hook" {
		t.Fatalf("workspace context called=%#v", called)
	}
	called = nil
	if _, err := dispatcher.PreToolUse(context.Background(), HookEvent{Provenance: HookProvenance{ExecutionID: "call_3", Origin: HookOriginAgent}, Tool: "run_command", WorkspaceID: "ws_other"}); err != nil {
		t.Fatal(err)
	}
	if len(called) != 1 || called[0] != "global-hook" {
		t.Fatalf("other workspace called=%#v", called)
	}
}
