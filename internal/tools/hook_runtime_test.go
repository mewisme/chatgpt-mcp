package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestRuntimePreHookCannotBypassDestructiveGuard(t *testing.T) {
	runtime, workspaceID := newApprovalShellRuntime(t)
	attachTestHooks(t, runtime, []pluginpkg.Capability{pluginpkg.CapabilityHookPreToolUse}, func(_ context.Context, _ pluginpkg.CapabilityProvider, event pluginpkg.HookEvent) (pluginpkg.HookResult, error) {
		if event.Type != pluginpkg.HookEventPreToolUse || event.SecurityCommand != "rm delete-me.txt" {
			t.Fatalf("pre hook event = %#v", event)
		}
		return pluginpkg.HookResult{Schema: pluginpkg.HookSchema, Decision: pluginpkg.HookDecisionContinue}, nil
	})
	item, err := runtime.Workspaces.Get(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(item.Path, "delete-me.txt")
	if err := os.WriteFile(target, []byte("protected"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Call(approvalContext("hook-destructive"), "run_command", map[string]any{"workspace_id": workspaceID, "command": "rm delete-me.txt"})
	if err != nil || !result.IsError {
		t.Fatalf("guard result = %#v err=%v", result, err)
	}
	challenge, ok := result.StructuredContent.(approvalRequiredResponse)
	if !ok || challenge.GuardCode != string(controlguard.CodeDestructiveMutation) {
		t.Fatalf("destructive challenge = %#v", result.StructuredContent)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("pre hook bypassed destructive guard: %v", err)
	}
}

func TestRuntimePreHookCannotBypassWorkspaceContainment(t *testing.T) {
	runtime, workspaceID := newApprovalShellRuntime(t)
	attachTestHooks(t, runtime, []pluginpkg.Capability{pluginpkg.CapabilityHookPreToolUse}, continueTestHook)
	outside := filepath.Join(t.TempDir(), "escape.txt")
	command := "touch " + outside
	result, err := runtime.Call(approvalContext("hook-containment"), "run_command", map[string]any{"workspace_id": workspaceID, "command": command})
	if err != nil || !result.IsError {
		t.Fatalf("containment result = %#v err=%v", result, err)
	}
	if _, ok := result.StructuredContent.(approvalRequiredResponse); ok {
		t.Fatalf("workspace containment became approvable: %#v", result.StructuredContent)
	}
	if _, err := os.Stat(outside); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre hook bypassed workspace containment: %v", err)
	}
}

func TestRuntimePreHookDenyAndFailureAreFailClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		runner pluginpkg.HookRunner
	}{
		{name: "deny", runner: func(context.Context, pluginpkg.CapabilityProvider, pluginpkg.HookEvent) (pluginpkg.HookResult, error) {
			return pluginpkg.HookResult{Schema: pluginpkg.HookSchema, Decision: pluginpkg.HookDecisionDeny, Reason: "policy blocked"}, nil
		}},
		{name: "failure", runner: func(context.Context, pluginpkg.CapabilityProvider, pluginpkg.HookEvent) (pluginpkg.HookResult, error) {
			return pluginpkg.HookResult{}, errors.New("hook crashed")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, workspaceID, called := newHookSyntheticRuntime(t)
			attachTestHooks(t, runtime, []pluginpkg.Capability{pluginpkg.CapabilityHookPreToolUse}, test.runner)
			result, err := runtime.Call(approvalContext("hook-"+test.name), "hook_target", map[string]any{"workspace_id": workspaceID})
			if err != nil || !result.IsError {
				t.Fatalf("result = %#v err=%v", result, err)
			}
			if called.Load() {
				t.Fatal("handler executed after pre hook blocked it")
			}
		})
	}
}

func TestRuntimePreHookProviderMetadataIsRecordedInCallObservation(t *testing.T) {
	runtime, workspaceID, _ := newHookSyntheticRuntime(t)
	attachTestHooks(t, runtime, []pluginpkg.Capability{pluginpkg.CapabilityHookPreToolUse}, continueTestHook)
	var start CallObservation
	runtime.CallObserver = func(observation CallObservation) {
		if observation.Phase == "start" {
			start = observation
		}
	}
	result, err := runtime.Call(context.Background(), "hook_target", map[string]any{"workspace_id": workspaceID})
	if err != nil || result.IsError {
		t.Fatalf("result = %#v err=%v", result, err)
	}
	plugins, ok := start.Raw["plugins"].(map[string]any)
	if !ok {
		t.Fatalf("plugin diagnostics = %#v", start.Raw["plugins"])
	}
	providers, ok := plugins["pre_tool_hooks"].([]pluginpkg.HookProviderMetadata)
	if !ok || len(providers) != 1 || providers[0].PluginID != "test-hook" || providers[0].Version != "1.0.0" || providers[0].Capability != pluginpkg.CapabilityHookPreToolUse {
		t.Fatalf("hook provider diagnostics = %#v", plugins["pre_tool_hooks"])
	}
}

func TestRuntimeHookDenialEmitsToolDeniedObservation(t *testing.T) {
	runtime, workspaceID, called := newHookSyntheticRuntime(t)
	denied := make(chan pluginpkg.HookEvent, 1)
	attachTestHooks(t, runtime, []pluginpkg.Capability{pluginpkg.CapabilityHookPreToolUse, pluginpkg.CapabilityHookToolDenied}, func(_ context.Context, _ pluginpkg.CapabilityProvider, event pluginpkg.HookEvent) (pluginpkg.HookResult, error) {
		if event.Type == pluginpkg.HookEventPreToolUse {
			return pluginpkg.HookResult{Schema: pluginpkg.HookSchema, Decision: pluginpkg.HookDecisionDeny, Reason: "blocked by hook"}, nil
		}
		if event.Type == pluginpkg.HookEventToolDenied {
			denied <- event
		}
		return pluginpkg.HookResult{Schema: pluginpkg.HookSchema, Decision: pluginpkg.HookDecisionContinue}, nil
	})
	result, err := runtime.Call(approvalContext("hook-denied-observe"), "hook_target", map[string]any{"workspace_id": workspaceID})
	if err != nil || !result.IsError || called.Load() {
		t.Fatalf("result = %#v called=%t err=%v", result, called.Load(), err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runtime.Hooks.WaitIdle(waitCtx); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-denied:
		if !strings.Contains(event.Error, "pre-tool hook denied execution") || event.Status != "denied" {
			t.Fatalf("denied event = %#v", event)
		}
	default:
		t.Fatal("hook denial did not emit ToolDenied")
	}
}

func TestRuntimePreHookRequireApprovalUsesCoreApprovalFlow(t *testing.T) {
	runtime, workspaceID, called := newHookSyntheticRuntime(t)
	attachTestHooks(t, runtime, []pluginpkg.Capability{pluginpkg.CapabilityHookPreToolUse}, func(context.Context, pluginpkg.CapabilityProvider, pluginpkg.HookEvent) (pluginpkg.HookResult, error) {
		return pluginpkg.HookResult{Schema: pluginpkg.HookSchema, Decision: pluginpkg.HookDecisionRequireApproval, Reason: "human review"}, nil
	})
	ctx := approvalContext("hook-approval")
	args := map[string]any{"workspace_id": workspaceID}
	guarded, err := runtime.Call(ctx, "hook_target", args)
	if err != nil || !guarded.IsError || called.Load() {
		t.Fatalf("guarded = %#v called=%t err=%v", guarded, called.Load(), err)
	}
	challenge, ok := guarded.StructuredContent.(approvalRequiredResponse)
	if !ok || challenge.GuardCode != string(controlguard.CodeHookPolicy) || challenge.TargetTool != "hook_target" {
		t.Fatalf("hook approval challenge = %#v", guarded.StructuredContent)
	}
	request, _, err := runtime.Approvals.CreateRequestWithTitle(challenge.ChallengeID, "hook-approval", workspaceID, "Allow hook target")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Approvals.Approve(request.ID, "test", "approved hook policy"); err != nil {
		t.Fatal(err)
	}
	approved, err := runtime.Call(ctx, "hook_target", args)
	if err != nil || approved.IsError || !called.Load() {
		t.Fatalf("approved = %#v called=%t err=%v", approved, called.Load(), err)
	}
}

func TestRuntimeObservationHooksReceiveLifecycleMetadataOnly(t *testing.T) {
	runtime, workspaceID, _ := newHookSyntheticRuntime(t)
	schema := Schema{Name: "hook_error", InputSchema: json.RawMessage(`{"type":"object","properties":{"workspace_id":{"type":"string"}},"required":["workspace_id"],"additionalProperties":false}`)}
	runtime.Registry.MustRegister("hook_error", schema, func(context.Context, map[string]any) (Result, error) {
		return Result{}, errors.New("tool failed")
	})
	schema.Name = "hook_denied"
	runtime.Registry.MustRegister("hook_denied", schema, func(context.Context, map[string]any) (Result, error) {
		return Result{}, controlguard.New(controlguard.CodeProtectedState, "tool denied", false, nil)
	})
	var mu sync.Mutex
	events := []pluginpkg.HookEvent{}
	attachTestHooks(t, runtime, []pluginpkg.Capability{pluginpkg.CapabilityHookPostToolUse, pluginpkg.CapabilityHookToolError, pluginpkg.CapabilityHookToolDenied}, func(_ context.Context, _ pluginpkg.CapabilityProvider, event pluginpkg.HookEvent) (pluginpkg.HookResult, error) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		return pluginpkg.HookResult{Schema: pluginpkg.HookSchema, Decision: pluginpkg.HookDecisionContinue}, nil
	})
	ctx := WithCallSource(context.Background(), "http")
	result, err := runtime.Call(ctx, "hook_target", map[string]any{"workspace_id": workspaceID})
	if err != nil || result.IsError {
		t.Fatalf("result = %#v err=%v", result, err)
	}
	errorResult, err := runtime.Call(ctx, "hook_error", map[string]any{"workspace_id": workspaceID})
	if err != nil || !errorResult.IsError {
		t.Fatalf("error result = %#v err=%v", errorResult, err)
	}
	deniedResult, err := runtime.Call(ctx, "hook_denied", map[string]any{"workspace_id": workspaceID})
	if err != nil || !deniedResult.IsError {
		t.Fatalf("denied result = %#v err=%v", deniedResult, err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runtime.Hooks.WaitIdle(waitCtx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 3 {
		t.Fatalf("events = %#v", events)
	}
	byType := map[pluginpkg.HookEventType]pluginpkg.HookEvent{}
	for _, event := range events {
		byType[event.Type] = event
	}
	post := byType[pluginpkg.HookEventPostToolUse]
	if post.Provenance.Origin != pluginpkg.HookOriginAgent || post.Provenance.ExecutionID == "" || post.Result == nil || post.Result.IsError || post.Result.ContentCount == 0 {
		t.Fatalf("post hook event = %#v", post)
	}
	toolError := byType[pluginpkg.HookEventToolError]
	if toolError.Error != "tool failed" || toolError.Status != "error" {
		t.Fatalf("tool error event = %#v", toolError)
	}
	denied := byType[pluginpkg.HookEventToolDenied]
	if denied.Error != "tool denied" || denied.Status != "denied" {
		t.Fatalf("tool denied event = %#v", denied)
	}
}

func TestRuntimeObservationHookFailureIsFailOpen(t *testing.T) {
	runtime, workspaceID, _ := newHookSyntheticRuntime(t)
	attachTestHooks(t, runtime, []pluginpkg.Capability{pluginpkg.CapabilityHookPostToolUse}, func(context.Context, pluginpkg.CapabilityProvider, pluginpkg.HookEvent) (pluginpkg.HookResult, error) {
		return pluginpkg.HookResult{}, errors.New("observer failed")
	})
	result, err := runtime.Call(context.Background(), "hook_target", map[string]any{"workspace_id": workspaceID})
	if err != nil || result.IsError {
		t.Fatalf("observation failure affected tool result: %#v err=%v", result, err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runtime.Hooks.WaitIdle(waitCtx); err != nil {
		t.Fatal(err)
	}
	if runtime.Hooks.Stats().Failed == 0 {
		t.Fatal("observation hook failure was not recorded")
	}
}

func newHookSyntheticRuntime(t *testing.T) (*Runtime, string, *atomic.Bool) {
	t.Helper()
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := manager.Instance()
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	called := &atomic.Bool{}
	registry.MustRegister("hook_target", Schema{Name: "hook_target", InputSchema: json.RawMessage(`{"type":"object","properties":{"workspace_id":{"type":"string"}},"required":["workspace_id"],"additionalProperties":false}`)}, func(context.Context, map[string]any) (Result, error) {
		called.Store(true)
		return TextResult("ok"), nil
	})
	runtime := &Runtime{Registry: registry, Workspaces: manager, SessionAccess: NewSessionWorkspaceAccessManager(), Approvals: approval.NewManager(identity.ID)}
	RegisterApprovalTools(registry, runtime)
	return runtime, item.ID, called
}

func attachTestHooks(t *testing.T, runtime *Runtime, capabilities []pluginpkg.Capability, runner pluginpkg.HookRunner) {
	t.Helper()
	root := t.TempDir()
	layout := pluginpkg.Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{OS: goruntime.GOOS, Arch: goruntime.GOARCH, CoreVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(root, "payload")
	if err := os.MkdirAll(filepath.Join(payload, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, "bin", "hook"), []byte("hook"), 0700); err != nil {
		t.Fatal(err)
	}
	permissions := []pluginpkg.Permission{pluginpkg.PermissionProcessExecute}
	for _, capability := range capabilities {
		if capability == pluginpkg.CapabilityHookPreToolUse {
			permissions = appendPermission(permissions, pluginpkg.PermissionHookToolControl)
		} else {
			permissions = appendPermission(permissions, pluginpkg.PermissionHookToolObserve)
		}
	}
	manifest := pluginpkg.Manifest{
		Schema: pluginpkg.ManifestSchema, ID: "test-hook", Name: "Test Hook", Publisher: "mewisme", License: "Apache-2.0", Version: "1.0.0", Type: "hook", Provides: capabilities, Permissions: permissions,
		Platforms: map[string]pluginpkg.PlatformArtifact{goruntime.GOOS + "/" + goruntime.GOARCH: {Artifact: "test-hook.tar.gz", SHA256: strings.Repeat("a", 64), Archive: "tar.gz", Entrypoint: "bin/hook"}},
	}
	if _, err := store.Install(manifest, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("test-hook", "1.0.0", pluginpkg.ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	runtime.Hooks = pluginpkg.NewHookDispatcherWithRunner(store, runner)
}

func appendPermission(values []pluginpkg.Permission, value pluginpkg.Permission) []pluginpkg.Permission {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func continueTestHook(context.Context, pluginpkg.CapabilityProvider, pluginpkg.HookEvent) (pluginpkg.HookResult, error) {
	return pluginpkg.HookResult{Schema: pluginpkg.HookSchema, Decision: pluginpkg.HookDecisionContinue}, nil
}
