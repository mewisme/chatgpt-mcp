package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOfficialRTKManifestUsesDeclarativeHostContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "rtk", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := manifest.Platform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "rtk" || !artifact.HostBacked() || artifact.Host.CommandWrapper == nil {
		t.Fatalf("official RTK host contract = %#v", artifact.Host)
	}
	if len(artifact.Host.Install) == 0 || len(artifact.Host.Checks) < 2 {
		t.Fatalf("official RTK host metadata = %#v", artifact.Host)
	}
	windows := manifest.Platforms["windows/amd64"]
	if windows.Host == nil || !hasInstallHint(windows.Host.Install, "winget install rtk-ai.rtk") {
		t.Fatalf("official RTK Windows install hints = %#v", windows.Host)
	}
	t.Setenv("PATH", t.TempDir())
	_, err = preflightHostExecutable(context.Background(), artifact)
	if err == nil || !strings.Contains(err.Error(), "Recommended installation commands:") || !strings.Contains(err.Error(), "cargo install --git https://github.com/rtk-ai/rtk --branch master rtk") {
		t.Fatalf("official RTK missing-host remediation = %v", err)
	}
}

func TestHostBackedManifestValidation(t *testing.T) {
	manifest := testHostWrapperManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	artifact, err := manifest.Platform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if !artifact.HostBacked() || artifact.Host.CommandWrapper == nil {
		t.Fatalf("host artifact = %#v", artifact)
	}
	artifact.Artifact = "wrap.zip"
	manifest.Platforms[runtime.GOOS+"/"+runtime.GOARCH] = artifact
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "cannot mix") {
		t.Fatalf("mixed host/package manifest accepted: %v", err)
	}
}

func TestManagerHostBackedInstallPreflightRecommendsInstallCommands(t *testing.T) {
	manifest := testHostWrapperManifest()
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	assets := map[string][]byte{"wrap-1.0.0.json": manifestData, "wrap-1.0.0.json.sigstore.json": []byte(`{"signature":"manifest"}`)}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		data, ok := assets[strings.TrimPrefix(request.URL.Path, "/")]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write(data)
	}))
	defer server.Close()
	layout := testLayout(t)
	store, err := NewStore(layout, RuntimeContext{OS: runtime.GOOS, Arch: runtime.GOARCH, CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	publisher := Publisher{Name: "mewisme", Trusted: true, Sigstore: SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}}
	resolved := ResolvedPlugin{Registry: Registry{Name: "test", URL: server.URL, Trust: &publisher.Sigstore}, PluginID: "wrap", Version: "1.0.0", Publisher: publisher, ManifestName: "wrap-1.0.0.json"}
	config := NewConfig()
	config.Registries["test"] = resolved.Registry
	if err := WriteConfig(layout.ConfigPath(), config); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store, RegistryClient: RegistryClient{HTTPClient: server.Client(), Layout: layout, Verifier: testRegistryVerifier}, HTTPClient: server.Client()}
	t.Setenv("PATH", t.TempDir())
	_, err = manager.installResolved(context.Background(), resolved, false, true)
	if err == nil || !strings.Contains(err.Error(), "pre-install check failed") || !strings.Contains(err.Error(), "pkg install wrap") {
		t.Fatalf("missing host prerequisite error = %v", err)
	}
	if _, err := os.Stat(layout.InstalledVersionPath("wrap", "1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("failed preflight mutated installed state: %v", err)
	}
	fake := writeFakeHostWrapper(t)
	t.Setenv("PATH", filepath.Dir(fake))
	result, err := manager.installResolved(context.Background(), resolved, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Plugin.Entrypoint != fake || result.Plugin.Host == nil || result.Plugin.Payload != "" {
		t.Fatalf("host-backed install = %#v", result.Plugin)
	}
	if _, err := os.Stat(filepath.Join(result.Plugin.Root, "payload")); !os.IsNotExist(err) {
		t.Fatalf("host-backed plugin unexpectedly installed payload: %v", err)
	}
}

func TestHostPreflightRejectsFailedDeclarativeCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake host fixture")
	}
	root := t.TempDir()
	path := filepath.Join(root, "wrap")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root)
	artifact := testHostWrapperManifest().Platforms[runtime.GOOS+"/"+runtime.GOARCH]
	if _, err := preflightHostExecutable(context.Background(), artifact); err == nil || !strings.Contains(err.Error(), "identity") || !strings.Contains(err.Error(), "pkg install wrap") {
		t.Fatalf("preflight error = %v", err)
	}
}

func TestEnablingHostBackedPluginRevalidatesExecutable(t *testing.T) {
	fake := writeFakeHostWrapper(t)
	t.Setenv("PATH", filepath.Dir(fake))
	store := testStore(t)
	manifest := testHostWrapperManifest()
	if _, err := store.Install(manifest, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateWithState("wrap", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}, false); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	if err := store.SetEnabled("wrap", true); err == nil || !strings.Contains(err.Error(), "pkg install wrap") {
		t.Fatalf("enable error = %v", err)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if lock.Plugins["wrap"].Enabled {
		t.Fatal("failed host preflight enabled plugin")
	}
}

func TestReconcileDisablesHostBackedPluginWhenExecutableDisappears(t *testing.T) {
	fake := writeFakeHostWrapper(t)
	t.Setenv("PATH", filepath.Dir(fake))
	store := testStore(t)
	manifest := testHostWrapperManifest()
	if _, err := store.Install(manifest, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("wrap", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	report, err := Reconcile(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Disabled) != 1 || report.Disabled[0] != "wrap" || !strings.Contains(report.Issues["wrap"], "not installed or not on PATH") {
		t.Fatalf("reconcile report = %#v", report)
	}
}

func hasInstallHint(hints []HostInstallHint, command string) bool {
	for _, hint := range hints {
		if hint.Command == command {
			return true
		}
	}
	return false
}

func testHostWrapperManifest() Manifest {
	executable := "wrap"
	if runtime.GOOS == "windows" {
		executable = "wrap.exe"
	}
	return Manifest{
		Schema: ManifestSchema, ID: "wrap", Name: "Host Wrapper", Publisher: "mewisme", Version: "1.0.0", Type: "command-wrapper",
		Requires: Requirements{ChatGPTMCP: ">=0.2.24"}, Provides: []Capability{"command-wrapper/wrap"}, Permissions: []Permission{PermissionProcessExecute},
		Platforms: map[string]PlatformArtifact{runtime.GOOS + "/" + runtime.GOARCH: {Host: &HostExecutableSpec{
			Executable:     executable,
			Checks:         []HostExecutableCheck{{Name: "identity", Args: []string{"check"}, StdoutContains: "ready"}},
			Install:        []HostInstallHint{{Label: "Package manager", Command: "pkg install wrap"}},
			CommandWrapper: &HostCommandWrapper{Args: []string{"rewrite", "{command}"}, RewriteExitCodes: []int{0, 3}, PassthroughExitCodes: []int{1, 2}},
		}}},
	}
}

func writeFakeHostWrapper(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake host fixture")
	}
	path := filepath.Join(t.TempDir(), "wrap")
	script := "#!/bin/sh\nif [ \"$1\" = \"check\" ]; then echo 'ready'; exit 0; fi\nif [ \"$1\" = \"rewrite\" ]; then case \"$2\" in 'git status') echo 'wrap git status'; exit 0;; 'git push --force') echo 'wrap git push --force'; exit 3;; 'cat file.txt') echo 'wrap read file.txt'; exit 0;; *) exit 1;; esac; fi\nexit 64\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
