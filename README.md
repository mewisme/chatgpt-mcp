# chatgpt-mcp

[![CI](https://github.com/mewisme/chatgpt-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/mewisme/chatgpt-mcp/actions/workflows/ci.yml)
[![Release](https://github.com/mewisme/chatgpt-mcp/actions/workflows/release.yml/badge.svg)](https://github.com/mewisme/chatgpt-mcp/actions/workflows/release.yml)

A self-hosted MCP 2026-07-28 runtime written in Go with an embedded React administration dashboard and a builtin OpenAI Secure MCP Tunnel client.

`chatgpt-mcp` provides one cross-platform binary for local workspace tools, upstream MCP aggregation, OpenAI tunnel connectivity, authentication, activity monitoring, and administration.

## Features

- Stateless MCP 2026-07-28 HTTP runtime at `/mcp`
- `server/discover`, `tools/list`, `tools/call`, and `subscriptions/listen`
- Per-request protocol version, client capabilities, and client metadata
- SEP-2243 `Mcp-Method`, `Mcp-Name`, and `Mcp-Param-*` validation
- Multi Round-Trip Requests (MRTR) with `input_required`, `inputRequests`, `inputResponses`, and `requestState`
- Workspace-bound filesystem, shell, Git, context, rules, skills, checkpoint, and utility tools
- Dynamic upstream MCP server aggregation with OAuth and MRTR relay
- Builtin OpenAI Secure MCP Tunnel using `github.com/openai/tunnel-client`
- Embedded React admin dashboard
- MCP and admin bearer-token authentication
- Activity stream and audit events
- Cross-platform single-binary releases for Linux, Windows, and macOS

## Requirements

For source builds:

- Go 1.27+
- Node.js 24+
- pnpm 11+

Binary releases do not require Node.js or pnpm.

## Installation

### Direct installer

Linux/macOS:

```bash
curl -fsSL get.mewis.me/chatgpt-mcp.sh | sh
```

Windows PowerShell:

```powershell
irm https://get.mewis.me/chatgpt-mcp.ps1 | iex
```

Install a specific release:

```bash
curl -fsSL get.mewis.me/chatgpt-mcp.sh | env CHATGPT_MCP_VERSION=v0.1.0 sh
```

```powershell
$env:CHATGPT_MCP_VERSION = 'v0.1.0'
irm https://get.mewis.me/chatgpt-mcp.ps1 | iex
```

The Unix installer keeps versions under `~/.chatgpt-mcp` and links both `chatgpt-mcp` and the short alias `cmcp` into `~/.local/bin`. The Windows installer uses `%LOCALAPPDATA%\chatgpt-mcp\current`, adds that directory to the user `PATH`, and installs `cmcp` as a command shim for the same executable.

Unix uninstall:

```bash
curl -fsSL get.mewis.me/chatgpt-mcp.sh | sh -s -- --uninstall
```

Windows uninstall:

```powershell
& ([scriptblock]::Create((irm https://get.mewis.me/chatgpt-mcp.ps1))) -Uninstall
```

### Homebrew

```bash
brew tap mewisme/mew
brew install --cask chatgpt-mcp
```

The generated cask is synchronized through `mewisme/homebrew-mew`.

Both `chatgpt-mcp` and `cmcp` are installed as commands.

### Scoop

```powershell
scoop bucket add mew https://github.com/mewisme/scoop-mew
scoop install mew/chatgpt-mcp
```

The Scoop manifest creates shims for both `chatgpt-mcp` and `cmcp`.

### Local source install

From a cloned repository:

```bash
node scripts/install-local.mjs
```

The installer restores frontend dependencies, builds the dashboard, prepares `internal/web/dist`, then runs:

```bash
go install .
```

It also creates `cmcp` beside the installed Go binary (`cmcp` symlink on Unix, `cmcp.cmd` shim on Windows).

Useful variants:

```bash
node scripts/install-local.mjs --no-deps
node scripts/install-local.mjs --from-dist
```

### Binary release

Download a release artifact for one of these targets:

- Linux amd64/arm64
- Windows amd64/arm64
- macOS amd64/arm64

Then run:

```bash
chatgpt-mcp --version
```

## Quick start

Initialize the local configuration:

```bash
chatgpt-mcp init
```

JSON is the default storage format. YAML and TOML are also supported:

```bash
chatgpt-mcp init --json
chatgpt-mcp init --yaml
chatgpt-mcp init --toml
chatgpt-mcp init --format toml
```

The command creates MCP/admin credentials and stores configuration under:

```text
~/.config/chatgpt-mcp/
```

For test binaries, CI, or parallel isolated instances, override the entire persistent config/state root with `--config-dir`:

```bash
chatgpt-mcp --config-dir ./.tmp/chatgpt-mcp-test init
chatgpt-mcp --config-dir ./.tmp/chatgpt-mcp-test serve
```

The equivalent environment variable is `CHATGPT_MCP_CONFIG_DIR`. The CLI flag takes precedence over the environment variable, which takes precedence over the default `~/.config/chatgpt-mcp` root. The override applies to config, tunnel secrets, OAuth/upstream state, workspaces, shell state, memory, checkpoints, and other persistent runtime state, so test runs do not mutate the normal user instance.

Start the runtime:

```bash
chatgpt-mcp serve
```

Default endpoints:

```text
MCP:   http://127.0.0.1:37421/mcp
Admin: http://127.0.0.1:37422/
```

Network exposure is explicit and independent from authentication:

```json
{
  "server": {
    "port": 37421,
    "expose": {
      "mode": "none",
      "interfaces": []
    }
  }
}
```

`none` binds only to `127.0.0.1`. `all` binds to `0.0.0.0`. `interfaces` keeps loopback available and additionally binds only to the eligible IPv4 addresses of the selected active interfaces.

For one run:

```bash
chatgpt-mcp serve --expose
chatgpt-mcp serve --expose=all
chatgpt-mcp serve --expose=eth0
chatgpt-mcp serve --expose=eth0,tailscale0
chatgpt-mcp serve --expose=none
```

Bare `--expose` means `all`; `--expose=true` and `--expose=false` remain compatibility aliases for `all` and `none`.

Persist the policy with:

```bash
chatgpt-mcp config set server.expose all
chatgpt-mcp config set server.expose eth0,tailscale0
chatgpt-mcp config set server.expose none
```

Selected interfaces must be active, non-loopback, and have an eligible IPv4 address. Startup fails instead of silently broadening exposure when a configured interface is unavailable. Duplicate IPs are bound once. Legacy boolean `server.expose` and `server.host` values are migrated automatically on load and written back in the structured exposure format on the next save.

Inspect the current runtime configuration:

```bash
chatgpt-mcp status
chatgpt-mcp config get
chatgpt-mcp config list
chatgpt-mcp config list admin
chatgpt-mcp config get admin.enabled
```

`config get` and `config list` use dotted output by default. Parent keys recursively list their children. Structured output can be selected independently from the on-disk format:

```bash
chatgpt-mcp config list --json
chatgpt-mcp config list --yaml
chatgpt-mcp config list --toml
chatgpt-mcp config list --format yaml
chatgpt-mcp config get admin --toml
```

Global filesystem access outside registered workspace roots is explicit through `permissions.allow_dirs`. Directories must be absolute and exist when configured. For example, allowing `/tmp` lets every workspace-bound Agent read/write test or build artifacts there:

```bash
chatgpt-mcp config allow-dir add /tmp
chatgpt-mcp config allow-dir list
chatgpt-mcp config allow-dir remove /tmp
```

The full list can also be replaced directly:

```bash
chatgpt-mcp config set permissions.allow_dirs /tmp,/var/tmp/chatgpt-mcp
```

Admin Settings exposes the same global allow list and applies changes to the live tool runtime after persistence succeeds; no runtime restart is required.

Sensitive keys keep their real names but their values are rendered as `<redacted>` by the config inspection commands. The active main config controls the serialization format of structured `chatgpt-mcp` state such as tunnel secrets, upstream servers, workspace registry, OAuth state, shell state, and rewind metadata. Append-only activity logs remain JSONL.

Convert the active configuration and structured state tree transactionally:

```bash
chatgpt-mcp config convert json
chatgpt-mcp config convert yaml
chatgpt-mcp config convert toml
chatgpt-mcp config transform toml
```

`convert` and `transform` are aliases. The command recursively converts every managed structured config/state file under the active config root (default `~/.config/chatgpt-mcp`), including mixed JSON/YAML/TOML state left from older versions, after a full preflight and with rollback on mutation failure.

Verify the full config/state tree before starting the runtime or after manual edits:

```bash
chatgpt-mcp config verify
chatgpt-mcp config validate
```

`verify` and `validate` are aliases. Verification checks that the main config exists, every managed structured file uses the same extension/format as the main config, every file decodes successfully, and the loaded runtime configuration passes semantic validation.

Remove all local `chatgpt-mcp` configuration and state:

```bash
chatgpt-mcp uninit
```

## MCP protocol

The HTTP endpoint implements the stateless MCP `2026-07-28` protocol revision.

Modern requests use `POST /mcp` and carry routing/version metadata on every request. A typical request includes:

```http
MCP-Protocol-Version: 2026-07-28
Mcp-Method: tools/list
Content-Type: application/json
```

with request metadata:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list",
  "params": {
    "_meta": {
      "io.modelcontextprotocol/protocolVersion": "2026-07-28",
      "io.modelcontextprotocol/clientCapabilities": {},
      "io.modelcontextprotocol/clientInfo": {
        "name": "example-client",
        "version": "1.0.0"
      }
    }
  }
}
```

There is no `initialize` handshake and no `Mcp-Session-Id` in this protocol revision. Removed or unknown MCP methods return HTTP `404` with JSON-RPC error `-32601`. `GET` and `DELETE` on the MCP endpoint return `405`.

## Authentication

`chatgpt-mcp` stores token hashes rather than plaintext MCP/admin tokens. Tokens are shown when they are created or rotated.

Create or rotate credentials:

```bash
chatgpt-mcp auth mcp-create
chatgpt-mcp auth admin-create
```

Use an enabled token with:

```http
Authorization: Bearer <token>
```

Authentication can be inspected with:

```bash
chatgpt-mcp auth status
```

Authentication and network exposure are separate policies. When MCP or Admin authentication is disabled, that endpoint does not require a bearer token on either loopback or exposed interfaces. The Admin dashboard also skips the login screen when Admin authentication is disabled.

## OpenAI Secure MCP Tunnel

The OpenAI Secure MCP Tunnel is embedded directly in the Go binary. No `cloudflared`, `ngrok`, external tunnel executable, or public local MCP port is required.

Configure it with a tunnel ID and Runtime API key:

```bash
chatgpt-mcp tunnel configure \
  --enabled \
  --id tunnel_... \
  --api-key <runtime-api-key>
```

Optional OpenAI-specific settings:

```bash
chatgpt-mcp tunnel configure \
  --control-plane-base-url https://api.openai.com \
  --organization-id org_...
```

Check configuration/runtime status:

```bash
chatgpt-mcp tunnel status
```

Enable or disable automatic tunnel startup:

```bash
chatgpt-mcp tunnel enable
chatgpt-mcp tunnel disable
```

Run only the builtin tunnel in the foreground:

```bash
chatgpt-mcp tunnel run
```

Unexpected tunnel runtime or embedded MCP transport failures are supervised automatically. The runtime emits a degraded state, reconnects with bounded exponential backoff (`1s`, `2s`, `4s`, up to `30s`), and returns to ready when the replacement connection succeeds. Explicit stop/disable/reconfigure and process shutdown cancel the supervisor, so they never trigger an automatic restart.

The tunnel API key is kept separately from the main config using the same storage format, for example:

```text
~/.config/chatgpt-mcp/config.toml
~/.config/chatgpt-mcp/tunnel.toml
```

Tunnel configuration updates are transactional. Main configuration and the separate tunnel secret are rolled back together on persistence failure, and Admin API reconfiguration restores the previous runtime configuration/running state if applying or persisting the candidate fails.

## Upstream MCP servers

`chatgpt-mcp` can connect to other MCP servers and expose enabled upstream tools through the same local runtime.

Management is available through:

```bash
chatgpt-mcp mcp --help
```

and the admin dashboard.

Upstream HTTP OAuth credentials are stored separately from normal runtime configuration. Upstream tool changes propagate into the exposed tool catalog and MCP subscriptions.

Upstream proxy refreshes are atomic. A refresh builds the complete replacement proxy set first and commits it in one registry swap only after every required upstream succeeds. Transient discovery or schema failures keep the previous proxy catalog intact, and Admin API mutations report refresh failures instead of silently discarding them.

Runtime configuration is exposed through a synchronized immutable-snapshot store. Admin configuration, presets, tunnel reconfiguration, and request-time authentication read or update the same store; updates serialize validation, persistence, and in-memory commit to avoid races and lost updates. Linux CI also runs the Go race detector.

Built-in agent features live under `features`. Ponytail and Caveman are enabled by default, can be toggled independently with `config set features.<name>.enabled <true|false>` or Admin Settings, and update the live tool catalog when changed through the Admin API. Caveman is a built-in terse-response turn controller; Ponytail continues to use its trusted plugin hooks when its feature is enabled.

Admin Activity streaming uses sequenced SSE events with `id` values, an initial `ready` control event, periodic heartbeats, and explicit overflow termination for slow subscribers. The Activity UI detects sequence gaps instead of silently hiding dropped events. Tunnel lifecycle transitions are emitted from the tunnel runtime itself as `connecting`, `degraded`, `reconnecting`, `ready`, and `stopped`, with the same transition feeding both runtime logs and Activity.

## Workspaces

Filesystem, shell, and Git mutations are workspace-bound. Register a workspace before using tools that operate on local project files:

```bash
chatgpt-mcp workspace --help
```

Workspace handles are explicit and immutable; tool calls cannot silently switch to another workspace.

A workspace can also grant its own additional directories without exposing them to every other workspace:

```bash
chatgpt-mcp workspace allow-dir add ws_... /path/to/build-cache
chatgpt-mcp workspace allow-dir list ws_...
chatgpt-mcp workspace allow-dir remove ws_... /path/to/build-cache
```

The effective filesystem scope for an Agent is the workspace root plus global `permissions.allow_dirs` plus that workspace's `allow_dirs`. Filesystem reads/writes, shell mutation validation, Git/process working directories, and rewind validation use the same canonical root set. Symlink escapes remain denied. Filesystem mutations in allowed directories still create rewind checkpoints, and revoking a directory prevents old checkpoints from restoring files back into the revoked path. Agents can inspect effective roots with `list_allowed_directories` or `agent_status`, but no MCP tool can grant new directories to itself.

## CLI

```text
chatgpt-mcp
├── serve
├── init
├── uninit
├── status
├── config
├── auth
├── workspace
├── mcp
└── tunnel
```

Use `--help` on any command for the current flags and subcommands.

Global runtime isolation is available through `--config-dir <path>` or `CHATGPT_MCP_CONFIG_DIR=<path>`. Prefer it whenever running development/test binaries that execute mutating commands such as `init`, `uninit`, `config set`, workspace registration, or tunnel configuration.

### Logging

`chatgpt-mcp` uses CLI-first event output by default. Normal output shows only meaningful lifecycle and user-facing state with stable markers: `✓` success/ready, `!` warning, `×` error, `·` information, and `→` action. Context is rendered on indented lines instead of timestamp/level/component prefixes or long `key=value` tails.

Use `--verbose` for useful operational context such as tunnel IDs, bind/exposure details, route/channel/transport information, and tool-call completion metadata. Use `--debug` for the full diagnostic stream including timestamps, levels, components, event names, client IDs, TLS/proxy details, and raw dependency metadata. Low-value tunnel-client events such as poller startup, metadata fetches, or dispatcher registration stay debug-only.

Use `--log-format=json` for JSONL event output suitable for automation. JSON logging respects the selected visibility mode, so `--debug --log-format=json` emits the full diagnostic event stream.

```bash
chatgpt-mcp serve
chatgpt-mcp serve --verbose
chatgpt-mcp serve --debug
chatgpt-mcp serve --log-format=json
chatgpt-mcp serve --debug --log-format=json
```

## Development

Install frontend dependencies:

```bash
pnpm --dir web install
```

Run frontend checks:

```bash
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web typecheck
pnpm --dir web build
```

Prepare the ignored Go embed directory before compiling the backend:

```bash
node scripts/prepare-web-embed.mjs
```

Run backend verification:

```bash
go mod verify
go test ./...
go vet ./...
go build -trimpath ./
```

Run the native release smoke against a built binary:

```bash
go build -trimpath -o chatgpt-mcp ./
node scripts/smoke-release.mjs ./chatgpt-mcp
```

The smoke uses an isolated temporary home plus an explicit `--config-dir`, and verifies init/config/status, HTTP health, MCP discovery, tool listing, modern error behavior, shutdown, and uninit without touching the normal user config root.

## CI

Pushes to `main` and pull requests run:

- Web runtime/component smoke tests, lint, typecheck, and production build
- Native tests, vet, build, local-install smoke, and runtime/MCP E2E smoke on Linux, Windows, and macOS
- Cross-build validation for every release target:
  - linux/amd64
  - linux/arm64
  - windows/amd64
  - windows/arm64
  - darwin/amd64
  - darwin/arm64

Tag releases repeat the web and native gates before GoReleaser is allowed to publish.

## Release

Releases are produced by GoReleaser only after all native release jobs pass.

The repository is currently suitable for an initial `v0.1.0` release while the public API remains pre-1.0.

After the final release-readiness commit is on `main` and CI is green:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Release archives contain the standalone binary with the admin dashboard embedded, plus `README.md`, `LICENSE`, and `checksums.txt`.

GoReleaser also generates `chatgpt-mcp.json` for Scoop and `chatgpt-mcp.rb` for Homebrew Cask. The release workflow uploads both manifests as release assets and dispatches `sync-package` to `mewisme/scoop-mew` and `mewisme/homebrew-mew`.

Maintainers should configure the `PACKAGE_SYNC_TOKEN` repository secret with permission to dispatch workflows to both package-manager repositories. If the secret is absent, publishing still succeeds and package sync is skipped with a warning.

## License

MIT License. Copyright (c) 2026 Mew.
