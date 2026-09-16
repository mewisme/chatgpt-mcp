package plugindev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
)

var pluginBuilder = workflowBuild

var (
	prepareMu      sync.Mutex
	prepared       bool
	preparedLayout plugin.Layout
	preparedErr    error
)

func Prepare() (plugin.Layout, error) {
	if skipAutoDev(os.Getenv) {
		return plugin.DefaultLayout(), nil
	}
	prepareMu.Lock()
	defer prepareMu.Unlock()
	if prepared {
		return preparedLayout, preparedErr
	}
	preparedLayout, preparedErr = prepareLocked()
	prepared = true
	return preparedLayout, preparedErr
}

func RuntimeLayout() plugin.Layout {
	layout, _ := Prepare()
	if layout.ConfigRoot == "" {
		return plugin.DefaultLayout()
	}
	return layout
}

func Reset() {
	prepareMu.Lock()
	prepared = false
	preparedLayout = plugin.Layout{}
	preparedErr = nil
	prepareMu.Unlock()
}

func InspectLayout() plugin.Layout {
	if skipAutoDev(os.Getenv) {
		return plugin.DefaultLayout()
	}
	ctx, err := Detect()
	if err != nil || !ctx.Enabled {
		return plugin.DefaultLayout()
	}
	layout, err := Layout(ctx.Root, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return plugin.DefaultLayout()
	}
	return layout
}

func prepareLocked() (plugin.Layout, error) {
	ctx, err := Detect()
	if err != nil {
		return plugin.DefaultLayout(), err
	}
	if !ctx.Enabled {
		return plugin.DefaultLayout(), nil
	}
	return Bootstrap(ctx)
}

func skipAutoDev(getenv func(string) string) bool {
	if testing.Testing() {
		return true
	}
	return skipAutoDevEnv(getenv)
}

func skipAutoDevEnv(getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	if strings.TrimSpace(getenv("CHATGPT_MCP_PLUGIN_BUNDLE")) != "" {
		return true
	}
	return cheapArgs(os.Args[1:])
}

func cheapArgs(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--version", "--help", "-h", "version", "help", "completion", "__complete", "__completeNoDesc":
			return true
		}
	}
	return false
}

func Bootstrap(ctx Context) (plugin.Layout, error) {
	if !ctx.Enabled || ctx.Root == "" {
		return plugin.DefaultLayout(), nil
	}
	if strings.TrimSpace(os.Getenv("CHATGPT_MCP_PLUGIN_BUNDLE")) != "" {
		return plugin.DefaultLayout(), nil
	}
	layout, err := Layout(ctx.Root, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return plugin.Layout{}, fmt.Errorf("dev plugin discover: %w", err)
	}
	lock, err := AcquireBootstrapLock(ctx.Root)
	if err != nil {
		return layout, err
	}
	defer func() { _ = lock.Release() }()
	err = reconcile(ctx, layout)
	return layout, err
}

type Report struct {
	Built  []string
	Reused []string
	Failed []string
}

func (report Report) log() {
	if verboseDev() {
		for _, id := range report.Reused {
			fmt.Fprintf(os.Stderr, "DEV %s cached\n", id)
		}
		for _, id := range report.Built {
			fmt.Fprintf(os.Stderr, "DEV %s rebuilt\n", id)
		}
	}
	if len(report.Built) == 0 && len(report.Failed) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "DEV core plugins: built %s; reused %d cached\n", strings.Join(report.Built, ", "), len(report.Reused))
}

func verboseDev() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CHATGPT_MCP_LOG")))
	return value == "debug" || value == "verbose"
}

func reconcile(ctx Context, layout plugin.Layout) error {
	specs, err := LoadCorePlugins(ctx.Root)
	if err != nil {
		return err
	}
	store, err := plugin.NewStore(layout, plugin.RuntimeContext{CoreVersion: "dev"})
	if err != nil {
		return fmt.Errorf("dev plugin install: %w", err)
	}
	platform := runtime.GOOS + "/" + runtime.GOARCH
	report := Report{}
	var firstRequired error
	for _, spec := range specs {
		if !spec.AllowsPlatform(platform) {
			continue
		}
		built, err := ensurePlugin(ctx, store, spec, platform)
		if err != nil {
			report.Failed = append(report.Failed, spec.ID)
			if spec.Required && firstRequired == nil {
				firstRequired = err
			}
			continue
		}
		if built {
			report.Built = append(report.Built, spec.ID)
		} else {
			report.Reused = append(report.Reused, spec.ID)
		}
	}
	report.log()
	return firstRequired
}

func ensurePlugin(ctx Context, store *plugin.Store, spec CorePlugin, platform string) (bool, error) {
	fail := func(phase string, err error) error {
		return fmt.Errorf("dev plugin %s %s failed: %w (set %s=rebuild to rebuild or %s=off to disable)", spec.ID, phase, err, EnvPlugins, EnvPlugins)
	}
	fingerprint, err := Fingerprint(ctx.Root, spec.ID, platform)
	if err != nil {
		return false, fail("discover", err)
	}
	cache, err := CacheDir(ctx.Root, spec.ID, platform, fingerprint)
	if err != nil {
		return false, fail("discover", err)
	}
	built := ctx.SkipCache() || !Cached(cache, fingerprint)
	if built {
		parent := filepath.Dir(cache)
		stage, err := StageDir(parent)
		if err != nil {
			return false, fail("build", err)
		}
		if err := pluginBuilder(ctx.Root, spec.ID, stage); err != nil {
			_ = os.RemoveAll(stage)
			return false, fail("build", err)
		}
		if err := WriteCacheMarker(stage, fingerprint); err != nil {
			_ = os.RemoveAll(stage)
			return false, fail("build", err)
		}
		if err := PromoteDir(stage, cache); err != nil {
			return false, fail("build", err)
		}
	}
	manifest, extracted, err := extractCached(cache, spec.ID, platform)
	if err != nil {
		return false, fail("verify", err)
	}
	defer os.RemoveAll(extracted)
	if current, err := store.Installed(manifest.ID, manifest.Version); err == nil && !ctx.SkipCache() {
		if provenance, err := ReadProvenance(current); err == nil && provenance.SourceFingerprint == fingerprint {
			if err := store.ActivateWithState(manifest.ID, manifest.Version, LocalDevTrust(manifest.Publisher), spec.Enabled || spec.Required); err != nil {
				return false, fail("activate", err)
			}
			return false, nil
		}
	}
	if err := replaceInstalled(store, manifest.ID); err != nil {
		return false, fail("install", err)
	}
	installed, err := store.Install(manifest, extracted)
	if err != nil {
		return false, fail("install", err)
	}
	if err := store.ActivateWithState(manifest.ID, manifest.Version, LocalDevTrust(manifest.Publisher), spec.Enabled || spec.Required); err != nil {
		return false, fail("activate", err)
	}
	digest, err := fileDigest(filepath.Join(cache, manifest.Platforms[platform].Artifact))
	if err != nil {
		digest = fingerprint
	}
	if err := WriteProvenance(installed, Provenance{
		Schema: provenanceSchema, Origin: plugin.RegistryLocalDev, RepoRoot: ctx.Root, PluginID: spec.ID,
		SourceFingerprint: fingerprint, ArtifactDigest: digest,
	}); err != nil {
		return false, fail("install", err)
	}
	return built, nil
}

func replaceInstalled(store *plugin.Store, id plugin.PluginID) error {
	manager := plugin.Manager{Store: store}
	if err := manager.Uninstall(context.Background(), id, true); err != nil && !strings.Contains(err.Error(), "is not installed") {
		return err
	}
	return nil
}

func extractCached(cache, id, platform string) (plugin.Manifest, string, error) {
	matches, err := filepath.Glob(filepath.Join(cache, id+"-*.json"))
	if err != nil || len(matches) == 0 {
		return plugin.Manifest{}, "", fmt.Errorf("built manifest missing for %s", id)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		return plugin.Manifest{}, "", err
	}
	manifest, err := plugin.ParseManifest(data)
	if err != nil {
		return plugin.Manifest{}, "", err
	}
	if string(manifest.ID) != id {
		return plugin.Manifest{}, "", fmt.Errorf("built manifest id %s does not match %s", manifest.ID, id)
	}
	goos, goarch, _ := strings.Cut(platform, "/")
	artifact, err := manifest.Platform(goos, goarch)
	if err != nil {
		return plugin.Manifest{}, "", err
	}
	extracted, err := os.MkdirTemp(cache, "extract-")
	if err != nil {
		return plugin.Manifest{}, "", err
	}
	if err := plugin.ExtractArchive(filepath.Join(cache, artifact.Artifact), artifact.Archive, extracted); err != nil {
		_ = os.RemoveAll(extracted)
		return plugin.Manifest{}, "", err
	}
	return manifest, extracted, nil
}

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func workflowBuild(repoRoot, id, output string) error {
	script := filepath.Join(repoRoot, "scripts", "plugin-workflow.mjs")
	cmd := exec.Command("node", script, "build", id, output)
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), EnvPlugins+"="+ModeOff, "CGM_PLUGIN_UPX=off")
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || strings.Contains(err.Error(), "executable file not found") {
			return fmt.Errorf("node is required to build plugin %s", id)
		}
		return err
	}
	return nil
}
