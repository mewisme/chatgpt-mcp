# Development

This guide covers source builds, verification, CI, release smoke tests, and release workflow expectations.

## Requirements

- Go 1.27+
- Node.js 24+
- pnpm 11+

## Install frontend dependencies

```bash
pnpm --dir web install
```

## Frontend checks

```bash
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web typecheck
pnpm --dir web build
```

## Prepare the embedded frontend

The Go binary embeds the built admin dashboard. Prepare the generated embed directory before backend builds that require it:

```bash
node scripts/prepare-web-embed.mjs
```

## Backend checks

Verify modules:

```bash
go mod verify
```

Run tests with an explicit isolated config root:

```bash
CHATGPT_MCP_CONFIG_DIR="$(mktemp -d)" go test ./...
```

Race detector:

```bash
CHATGPT_MCP_CONFIG_DIR="$(mktemp -d)" go test -race ./...
```

Vet:

```bash
go vet ./...
```

Build:

```bash
go build -trimpath ./
```

## Config isolation is a test invariant

Tests and smoke tests must not use the real default/global config directory.

Use either:

```text
--config-dir <isolated-temp>
```

or:

```text
CHATGPT_MCP_CONFIG_DIR=<isolated-temp>
```

The release smoke also creates a default-root sentinel and verifies that the selected test flow does not mutate it.

This rule applies especially to commands such as:

- `init`
- `uninit`
- `config set`
- `config convert`
- workspace registration/access changes
- tunnel configuration
- managed runtime tests

## Local source install

```bash
node scripts/install-local.mjs
```

The script prepares the web embed and runs the local Go installation flow.

Variants:

```bash
node scripts/install-local.mjs --no-deps
node scripts/install-local.mjs --from-dist
```

Both `chatgpt-mcp` and `cgm` are installed beside the Go binary (`cgm` is a symlink on Unix and a command shim on Windows).

## Release smoke

Build a native binary:

```bash
node scripts/prepare-web-embed.mjs
go build -trimpath -o chatgpt-mcp ./
```

Run:

```bash
node scripts/smoke-release.mjs ./chatgpt-mcp
```

The portable smoke verifies behavior such as:

- managed direct self-install and idempotent reinstall
- fresh `install --no-alias` preserves the canonical command without creating `cgm`
- isolated init/uninit
- config verify/convert/transform
- config/status commands
- live config reload
- listener rebind and failed-bind rollback
- foreground runtime health
- managed runtime metadata through the portable hidden service entrypoint
- persistent runtime log replay/filter/follow/clear
- MCP discovery and tool listing
- modern MCP error behavior
- clean stop/shutdown

Updater-specific native tests additionally verify:

- HTTPS archive/checksum download through the real downloader into managed activation
- checksum mismatch never stages or activates the target version
- same/latest no-op and explicit downgrade behavior
- install-method policy for direct/Homebrew/Scoop/Go/development/standalone installs
- managed restart/readiness, foreground preservation, and `--no-restart`
- automatic rollback plus previous-runtime restart on failed readiness

Control-approval native smoke additionally verifies:

- challenge creation requires a real approvable guard failure
- session/workspace binding and fake-challenge rejection
- challenge/request/retry/capability expiry behavior
- exact retry succeeds once; replay fails
- mismatch does not consume the valid grant/capability
- deny/cancel/expiry lifecycle
- hard-deny guards remain non-approvable
- MCP tool context cannot self-approve through `cgm request approve/deny`
- CLI plain/JSON/non-TTY interactive fallback
- Admin loopback and remote-auth policy

The portable runtime smoke also checks that `request_control_approval` is present in the MCP catalog, `cgm request list` reaches the running runtime, JSON suppresses the TUI, and `--interactive` fails clearly when no terminal is attached. Workspace and upstream list commands are checked with the same deterministic fallback behavior.

Updater and control-approval integration gates run in every native Linux, macOS, and Windows CI/release job. Cross-build jobs continue to compile all six release OS/architecture targets.

## Cross-platform builds

Release targets:

```text
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
windows/amd64
windows/arm64
```

Example compile checks:

```bash
GOOS=linux GOARCH=arm64 go build ./...
GOOS=darwin GOARCH=amd64 go build ./...
GOOS=windows GOARCH=amd64 go build ./...
```

Do not attempt to execute a cross-compiled test binary on the host OS; use native CI jobs for runtime tests and `go build` for cross-platform compile validation.

## CI

Pushes to `main` and pull requests run:

### Web checks

- test
- lint
- typecheck
- production build
- web artifact upload for native/cross-build jobs

### Native Linux

- installer validation
- module verification
- local install smoke
- package alias smoke
- control approval smoke
- Go tests
- race detector
- vet
- native build
- runtime/MCP smoke

### Native macOS

- Unix installer validation
- module verification
- local install/package alias smoke
- control approval smoke
- Go tests
- vet
- native build
- runtime smoke

### Native Windows

- PowerShell installer validation
- module verification
- local install/package alias smoke
- control approval smoke
- Go tests
- vet
- native build
- runtime smoke

### Cross-build matrix

All six release targets are compiled after the native/web prerequisites are available.

## Installer model

Unix installer layout uses immutable versions and stable current/command links:

```text
~/.chatgpt-mcp/versions/<version>/...
~/.chatgpt-mcp/current -> selected version
~/.local/bin/chatgpt-mcp -> stable current path
~/.local/bin/cgm -> stable current path
```

Windows uses versioned directories plus a stable `current` directory junction so upgrades do not overwrite the executable currently held open by a managed runtime.

Managed service definitions therefore keep a stable launcher path instead of pinning one version-specific executable.

Direct updates stage an immutable target version, verify the downloaded release checksum before activation, switch `current`, and keep the previous version until runtime readiness succeeds. The install-global passive update cache lives at:

```text
<install-root>/state/update.json
```

Normal commands only read a fresh cache; explicit update checks bypass it and query the release source.

## Release workflow

Releases are produced by GoReleaser after release-native checks pass.

The release archive contains the standalone binary with the embedded admin dashboard plus release metadata/files configured by GoReleaser.

GoReleaser also produces package-manager manifests used by:

- `mewisme/scoop-mew`
- `mewisme/homebrew-mew`

The release workflow can dispatch package synchronization using the repository `PACKAGE_SYNC_TOKEN`. If the secret is not configured, release publishing can still succeed while package synchronization is skipped/warned according to workflow behavior.

## Tagging a release

After `main` is clean and CI is green:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

Use the next semantic version appropriate for the release instead of copying this example unchanged.

## Useful checks before committing

```bash
git diff --check
CHATGPT_MCP_CONFIG_DIR="$(mktemp -d)" go test ./...
go vet ./...
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web typecheck
pnpm --dir web build
```

For changes affecting service behavior, tunnel connectivity, runtime logs, configuration, or MCP protocol behavior, also run the release smoke.
