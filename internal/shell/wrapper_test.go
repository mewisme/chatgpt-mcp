package shell

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func TestShellCommandWrapperExecutesEffectiveAndAuditsProjection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX wrapper fixture")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash unavailable")
	}
	manager, workspaceID, _ := newShellTestManager(t)
	resolver := NewProviderResolver(nil)
	resolver.goos = runtime.GOOS
	resolver.lookPath = func(string) (string, error) { return bash, nil }
	manager.providers = resolver
	store := installShellWrapper(t, "rtk")
	manager.SetCommandWrapperPipeline(pluginpkg.NewCommandWrapperPipelineWithRunner(store, shellTransparentWrapperRunner("rtk")))

	result, err := manager.Exec(context.Background(), workspaceID, `printf wrapped`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "wrapped" || result.Command != "rtk printf wrapped" {
		t.Fatalf("result = %#v", result)
	}
	executions := manager.Executions().List(workspaceID, 1)
	if len(executions) != 1 {
		t.Fatalf("executions = %#v", executions)
	}
	info := executions[0]
	if info.RequestedCommand != "printf wrapped" || info.EffectiveCommand != "rtk printf wrapped" || info.SecurityCommand != "printf wrapped" || info.Command != info.EffectiveCommand {
		t.Fatalf("command audit = %#v", info)
	}
	if info.WrapperCapability != "command-wrapper/rtk" || info.WrapperProvider != "plugin/rtk" || info.WrapperVersion != "1.0.0" {
		t.Fatalf("wrapper audit = %#v", info)
	}
}

func TestBackgroundCommandWrapperSharesExecutionPipeline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX wrapper fixture")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash unavailable")
	}
	manager, workspaceID, _ := newShellTestManager(t)
	resolver := NewProviderResolver(nil)
	resolver.goos = runtime.GOOS
	resolver.lookPath = func(string) (string, error) { return bash, nil }
	manager.providers = resolver
	store := installShellWrapper(t, "rtk")
	manager.SetCommandWrapperPipeline(pluginpkg.NewCommandWrapperPipelineWithRunner(store, shellTransparentWrapperRunner("rtk")))
	processes := NewProcessManagerWithExecutions(manager.workspaces, manager, manager.Executions())

	started, err := processes.Start(context.Background(), workspaceID, `printf background`)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, err := processes.Status(workspaceID, started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(status) == 1 && !status[0].Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background wrapper process did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	output, err := processes.Output(workspaceID, started.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if output.Stdout != "background" {
		t.Fatalf("output = %#v", output)
	}
	snapshot, err := manager.Executions().Get(workspaceID, started.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	info := snapshot.Execution
	if info.RequestedCommand != "printf background" || info.EffectiveCommand != "rtk printf background" || info.SecurityCommand != "printf background" || info.WrapperProvider != "plugin/rtk" {
		t.Fatalf("execution = %#v", info)
	}
}

func TestApprovedDirectControlPlaneCommandBypassesWrappers(t *testing.T) {
	store := installShellWrapper(t, "rtk")
	var calls atomic.Int32
	manager, _, _ := newShellTestManager(t)
	manager.SetCommandWrapperPipeline(pluginpkg.NewCommandWrapperPipelineWithRunner(store, func(context.Context, pluginpkg.CapabilityProvider, pluginpkg.CommandWrapperRequest) (pluginpkg.CommandWrapperResponse, error) {
		calls.Add(1)
		return pluginpkg.CommandWrapperResponse{}, errors.New("wrapper must not run")
	}))
	invocation := controlguard.Invocation{Program: "cgm", Args: []string{"update"}, Command: "cgm update"}
	ctx := controlguard.WithApproval(context.Background(), controlguard.Approval{RequestID: "req_test", Capability: "cap_test", Invocation: invocation})
	plan, err := manager.prepareCommand(ctx, "run_command", invocation.Command)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || plan.Effective != invocation.Command || plan.Security != invocation.Command || plan.WrapperProvider != "" {
		t.Fatalf("plan = %#v calls=%d", plan, calls.Load())
	}
}

func installShellWrapper(t *testing.T, name string) *pluginpkg.Store {
	t.Helper()
	root := t.TempDir()
	layout := pluginpkg.Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{OS: runtime.GOOS, Arch: runtime.GOARCH, CoreVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(root, "payload")
	if err := os.MkdirAll(filepath.Join(payload, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	entrypoint := filepath.Join(payload, "bin", name)
	if runtime.GOOS == "windows" {
		entrypoint += ".exe"
	}
	if runtime.GOOS != "windows" {
		if err := os.WriteFile(entrypoint, []byte("#!/bin/sh\nexec \"$@\"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	} else if err := os.WriteFile(entrypoint, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	entrypointRel := filepath.ToSlash(strings.TrimPrefix(entrypoint, payload+string(filepath.Separator)))
	manifest := pluginpkg.Manifest{
		Schema: pluginpkg.ManifestSchema, ID: pluginpkg.PluginID(name), Name: name, Publisher: "mewisme", Version: "1.0.0", Type: "command-wrapper",
		Provides: []pluginpkg.Capability{pluginpkg.Capability("command-wrapper/" + name)}, Permissions: []pluginpkg.Permission{pluginpkg.PermissionProcessExecute},
		Platforms: map[string]pluginpkg.PlatformArtifact{runtime.GOOS + "/" + runtime.GOARCH: {Artifact: name + ".tar.gz", SHA256: strings.Repeat("a", 64), Archive: "tar.gz", Entrypoint: entrypointRel}},
	}
	if _, err := store.Install(manifest, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(pluginpkg.PluginID(name), "1.0.0", pluginpkg.ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	return store
}

func shellTransparentWrapperRunner(name string) pluginpkg.CommandWrapperRunner {
	return func(_ context.Context, _ pluginpkg.CapabilityProvider, request pluginpkg.CommandWrapperRequest) (pluginpkg.CommandWrapperResponse, error) {
		switch request.Operation {
		case pluginpkg.WrapperOperationCanWrap:
			value := true
			return pluginpkg.CommandWrapperResponse{Schema: pluginpkg.WrapperSchema, CanWrap: &value}, nil
		case pluginpkg.WrapperOperationRewrite:
			return pluginpkg.CommandWrapperResponse{Schema: pluginpkg.WrapperSchema, Command: name + " " + request.Command}, nil
		case pluginpkg.WrapperOperationProject:
			return pluginpkg.CommandWrapperResponse{Schema: pluginpkg.WrapperSchema, Command: strings.TrimPrefix(request.Command, name+" ")}, nil
		default:
			return pluginpkg.CommandWrapperResponse{}, errors.New("unexpected wrapper operation")
		}
	}
}
