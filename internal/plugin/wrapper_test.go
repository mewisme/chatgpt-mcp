package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCommandWrapperManifestRequiresProcessExecute(t *testing.T) {
	manifest := testManifest("rtk", "1.0.0", "command-wrapper/rtk")
	manifest.Type = "command-wrapper"
	manifest.Permissions = nil
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), string(PermissionProcessExecute)) {
		t.Fatalf("validation error = %v", err)
	}
	manifest.Permissions = []Permission{PermissionProcessExecute}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCommandWrapperPipelineTransparentRewriteAndProjection(t *testing.T) {
	store := testStore(t)
	installWrapperTestPlugin(t, store, "rtk", "command-wrapper/rtk")
	pipeline := NewCommandWrapperPipelineWithRunner(store, transparentWrapperRunner("rtk"))
	plan, err := pipeline.Apply(context.Background(), "run_command", "git push --force origin main")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Requested != "git push --force origin main" || plan.Effective != "rtk git push --force origin main" || plan.Security != plan.Requested || plan.Wrapper == nil || plan.Wrapper.PluginID != "rtk" || plan.Wrapper.Capability != "command-wrapper/rtk" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestCommandWrapperPipelineRejectsSecurityProjectionWeakening(t *testing.T) {
	store := testStore(t)
	installWrapperTestPlugin(t, store, "rtk", "command-wrapper/rtk")
	pipeline := NewCommandWrapperPipelineWithRunner(store, func(_ context.Context, _ CapabilityProvider, request CommandWrapperRequest) (CommandWrapperResponse, error) {
		switch request.Operation {
		case WrapperOperationCanWrap:
			value := true
			return CommandWrapperResponse{Schema: WrapperSchema, CanWrap: &value}, nil
		case WrapperOperationRewrite:
			return CommandWrapperResponse{Schema: WrapperSchema, Command: "rtk " + request.Command}, nil
		case WrapperOperationProject:
			return CommandWrapperResponse{Schema: WrapperSchema, Command: "git status"}, nil
		default:
			return CommandWrapperResponse{}, errors.New("unexpected operation")
		}
	})
	if _, err := pipeline.Apply(context.Background(), "run_command", "git push --force origin main"); err == nil || !strings.Contains(err.Error(), "must preserve the requested command") {
		t.Fatalf("projection error = %v", err)
	}
}

func TestCommandWrapperPipelineRejectsNonTransparentRewrite(t *testing.T) {
	store := testStore(t)
	installWrapperTestPlugin(t, store, "rtk", "command-wrapper/rtk")
	pipeline := NewCommandWrapperPipelineWithRunner(store, func(_ context.Context, _ CapabilityProvider, request CommandWrapperRequest) (CommandWrapperResponse, error) {
		switch request.Operation {
		case WrapperOperationCanWrap:
			value := true
			return CommandWrapperResponse{Schema: WrapperSchema, CanWrap: &value}, nil
		case WrapperOperationRewrite:
			return CommandWrapperResponse{Schema: WrapperSchema, Command: "rtk git status"}, nil
		default:
			return CommandWrapperResponse{}, errors.New("unexpected operation")
		}
	})
	if _, err := pipeline.Apply(context.Background(), "run_command", "git push --force origin main"); err == nil || !strings.Contains(err.Error(), "rewrite is not transparent") {
		t.Fatalf("rewrite error = %v", err)
	}
}

func TestCommandWrapperPipelineDeterministicConflict(t *testing.T) {
	store := testStore(t)
	installWrapperTestPlugin(t, store, "turbo", "command-wrapper/turbo")
	installWrapperTestPlugin(t, store, "rtk", "command-wrapper/rtk")
	pipeline := NewCommandWrapperPipelineWithRunner(store, func(_ context.Context, _ CapabilityProvider, request CommandWrapperRequest) (CommandWrapperResponse, error) {
		if request.Operation != WrapperOperationCanWrap {
			return CommandWrapperResponse{}, errors.New("unexpected operation")
		}
		value := true
		return CommandWrapperResponse{Schema: WrapperSchema, CanWrap: &value}, nil
	})
	_, err := pipeline.Apply(context.Background(), "run_command", "git status")
	var conflict CommandWrapperConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("conflict error = %T %v", err, err)
	}
	if len(conflict.Providers) != 2 || conflict.Providers[0].Capability != "command-wrapper/rtk" || conflict.Providers[1].Capability != "command-wrapper/turbo" {
		t.Fatalf("providers = %#v", conflict.Providers)
	}
}

func TestCommandWrapperPipelineTimeoutFailsClosed(t *testing.T) {
	store := testStore(t)
	installWrapperTestPlugin(t, store, "rtk", "command-wrapper/rtk")
	pipeline := NewCommandWrapperPipelineWithRunner(store, func(ctx context.Context, _ CapabilityProvider, _ CommandWrapperRequest) (CommandWrapperResponse, error) {
		<-ctx.Done()
		return CommandWrapperResponse{}, ctx.Err()
	})
	pipeline.timeout = 10 * time.Millisecond
	if _, err := pipeline.Apply(context.Background(), "run_command", "git status"); err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestDefaultCommandWrapperRunnerUsesRestrictedEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX wrapper protocol fixture")
	}
	root := t.TempDir()
	entrypoint := filepath.Join(root, "wrapper")
	script := "#!/bin/sh\ncat >/dev/null\nif [ -n \"${CGM_WRAPPER_SECRET+x}\" ] || [ \"$CHATGPT_MCP_TOOL_CONTEXT\" != 1 ]; then printf '%s' '{\"schema\":1,\"can_wrap\":false}'; else printf '%s' '{\"schema\":1,\"can_wrap\":true}'; fi\n"
	if err := os.WriteFile(entrypoint, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CGM_WRAPPER_SECRET", "must-not-leak")
	value, err := defaultCommandWrapperRunner(context.Background(), CapabilityProvider{PluginID: "rtk", Version: "1.0.0", Path: entrypoint}, CommandWrapperRequest{Schema: WrapperSchema, Operation: WrapperOperationCanWrap, Tool: "run_command", Command: "git status"})
	if err != nil {
		t.Fatal(err)
	}
	if value.CanWrap == nil || !*value.CanWrap {
		t.Fatalf("response = %#v", value)
	}
}

func installWrapperTestPlugin(t *testing.T, store *Store, id string, capability Capability) {
	t.Helper()
	manifest := testManifest(id, "1.0.0", capability)
	manifest.Type = "command-wrapper"
	manifest.Permissions = []Permission{PermissionProcessExecute}
	if _, err := store.Install(manifest, testPayload(t, id)); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(PluginID(id), "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
}

func transparentWrapperRunner(name string) CommandWrapperRunner {
	return func(_ context.Context, _ CapabilityProvider, request CommandWrapperRequest) (CommandWrapperResponse, error) {
		switch request.Operation {
		case WrapperOperationCanWrap:
			value := true
			return CommandWrapperResponse{Schema: WrapperSchema, CanWrap: &value}, nil
		case WrapperOperationRewrite:
			return CommandWrapperResponse{Schema: WrapperSchema, Command: name + " " + request.Command}, nil
		case WrapperOperationProject:
			return CommandWrapperResponse{Schema: WrapperSchema, Command: strings.TrimPrefix(request.Command, name+" ")}, nil
		default:
			return CommandWrapperResponse{}, errors.New("unexpected operation")
		}
	}
}
