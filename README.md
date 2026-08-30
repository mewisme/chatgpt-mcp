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
- Persistent structured runtime journal, filtered CLI log replay, and live follow
- Managed background services for Linux, macOS, and Windows
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

The Unix installer keeps immutable versions under `~/.chatgpt-mcp/versions`, maintains `~/.chatgpt-mcp/current`, and links both `chatgpt-mcp` and the short alias `cmcp` into `~/.local/bin` through that stable current path. The Windows installer keeps versions under `%LOCALAPPDATA%\chatgpt-mcp\versions`, switches `%LOCALAPPDATA%\chatgpt-mcp\current` as a directory junction, adds that stable directory to the user `PATH`, and installs `cmcp` as a command shim for the same executable. Managed service definitions therefore keep a stable launcher path across upgrades instead of pinning a version-specific executable.

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

`serve` is the foreground form: it remains attached to the current terminal/session. For a managed background runtime, install and start the service for the selected config root:

```bash
chatgpt-mcp up
```

Stop and remove the managed service without deleting configuration, workspaces, checkpoints, or runtime logs:

```bash
chatgpt-mcp down
```

On Linux, normal `up`/`down` use `systemd --user`. `sudo chatgpt-mcp up` / `sudo chatgpt-mcp down` use a system-level systemd unit that starts with the machine, but the MCP process itself still runs as the invoking user from `SUDO_USER`, never as root. A user service warns when systemd lingering is disabled because the user manager may stop after the final login/SSH session ends; `chatgpt-mcp` never enables lingering automatically.

On macOS, normal commands use a LaunchAgent and `sudo` uses a LaunchDaemon whose `UserName` remains the invoking user. On Windows, `up` always uses a per-user Task Scheduler task with `InteractiveToken` and `LeastPrivilege`, even from an elevated terminal; it does not use LocalSystem or store the user's password.

Every service definition persists an absolute `--config-dir`, so it does not depend on the environment of a later login session. The normal precedence remains `--config-dir` > `CHATGPT_MCP_CONFIG_DIR` > default config root. For Linux/macOS system scope invoked through `sudo`, the default root belongs to the invoking user rather than `/root`; an explicit flag or environment override still wins.

Default endpoints:

```text
MCP:   http://127.0.0.1:37421/mcp
Admin: http://127.0.0.1:37422/
```

Network exposure is explicit. Authentication is normally independent, except wildcard `0.0.0.0` exposure requires both MCP and Admin authentication with configured tokens:

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

`none` binds only to `127.0.0.1`. `all` binds loopback plus each currently active eligible IPv4 address individually. `interfaces` keeps loopback available and additionally binds only to the eligible IPv4 addresses of the selected active interfaces. `0.0.0.0` creates a wildcard listener that accepts every IPv4 interface, including interfaces that appear after startup, and is rejected unless both MCP and Admin authentication are enabled with configured tokens.

For one run:

```bash
chatgpt-mcp serve --expose
chatgpt-mcp serve --expose=all
chatgpt-mcp serve --expose=0.0.0.0
chatgpt-mcp serve --expose=eth0
chatgpt-mcp serve --expose=eth0,tailscale0
chatgpt-mcp serve --expose=none
```

Bare `--expose` means `all`. For compatibility with the old boolean exposure setting, `--expose=true` maps to wildcard `0.0.0.0` and `--expose=false` maps to `none`.

Persist the policy with:

```bash
chatgpt-mcp config set server.expose all
chatgpt-mcp config set server.expose 0.0.0.0
chatgpt-mcp config set server.expose eth0,tailscale0
chatgpt-mcp config set server.expose none
```

Selected interfaces must be active, non-loopback, and have an eligible IPv4 address. Startup fails instead of silently broadening exposure when a configured interface is unavailable. Duplicate IPs are bound once. Legacy boolean `server.expose=true` and legacy `server.host=0.0.0.0` migrate to explicit wildcard mode; false/loopback migrate to `none`.

Inspect the current runtime configuration:

```bash
chatgpt-mcp status
chatgpt-mcp config get
chatgpt-mcp config list
chatgpt-mcp config list admin
chatgpt-mcp config get admin.enabled
```

`status` also reports whether the selected config root has a running foreground runtime or managed service, including service scope/backend, service identity, process ID, and runtime start information when available. A normal user can inspect a system-managed instance; only mutations such as system-scope `down` require the matching privilege.

`config set` persists changes to disk. When `serve` is already running, apply the persisted configuration without restarting the process:

```bash
chatgpt-mcp config set server.port 41021
chatgpt-mcp config set admin.port 41022
chatgpt-mcp config set server.expose tailscale0
chatgpt-mcp config reload
```

`config reload` uses a loopback-only local control channel tied to the active config root. Auth, feature flags, filesystem permissions, and tunnel settings are updated in the live runtime. Changes to `server.port`, `server.expose`, `admin.enabled`, or `admin.port` rebind the HTTP listeners inside the same process. Listener reload is transactional: if a new port/address cannot be bound, the previous listeners are restored and the process remains available. A foreground `serve --expose=...` command-line override remains authoritative across reloads. The command fails when no running runtime is associated with the selected config root.

`config get` and `config list` use dotted output by default. Parent keys recursively list their children. Structured output can be selected independently from the on-disk format:

```bash
chatgpt-mcp config list --json
chatgpt-mcp config list --yaml
chatgpt-mcp config list --toml
chatgpt-mcp config list --format yaml
chatgpt-mcp config get admin --toml
```

Global filesystem access outside registered workspace roots is explicit through `permissions.allow_dirs`. Directories must be absolute and exist when configured. Configure the full list directly:

```bash
chatgpt-mcp config set permissions.allow_dirs /tmp,/var/tmp/chatgpt-mcp
chatgpt-mcp config get permissions.allow_dirs
```

Admin Settings exposes the same global allow list and applies changes to the live tool runtime after persistence succeeds; no runtime restart is required.

Sensitive keys keep their real names but their values are rendered as `<redacted>` by the config inspection commands. The active main config controls the serialization format of structured `chatgpt-mcp` state such as tunnel secrets, upstream servers, workspace registry, OAuth state, shell state, and rewind metadata. Runtime events are persisted independently as a sanitized JSONL journal at `<config-root>/logs/runtime.jsonl`, rotated by default at 10 MiB with five files retained.

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
chatgpt-mcp auth mcp create
chatgpt-mcp auth admin create
```

Use an enabled token with:

```http
Authorization: Bearer <token>
```

Authentication can be inspected with:

```bash
chatgpt-mcp auth status
```

Authentication and network exposure are separate policies except for wildcard `0.0.0.0`, which requires both MCP and Admin authentication with configured tokens. For other exposure modes, disabling an auth policy removes the bearer-token requirement for that endpoint. The Admin dashboard skips the login screen when Admin authentication is disabled.

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

Shell/process tools mark descendants as MCP tool execution context. In that context the `chatgpt-mcp` CLI is fail-closed: only explicitly read-only commands such as `status`, `config get/list`, `auth status`, workspace inspection, upstream inspection, `tunnel status`, and runtime log reading/following are accepted. Control-plane mutations such as `up`, `down`, `_service`, `logs clear`, `config set`, `config reload`, auth changes, workspace registration/access grants, upstream changes, tunnel configuration, `init`, and `uninit` are denied. The shell policy also rejects direct `cmcp` / `chatgpt-mcp` mutation commands, including common wrappers and nested shells, and denies direct shell reads of the protected config/state subtree. An Agent therefore cannot use the built-in shell path to recover runtime control credentials or directly grant itself additional filesystem access.

This is defense-in-depth for the built-in tool runner, not an OS security boundary against arbitrary code running as the same operating-system user. Strong isolation against a deliberately hostile local process requires an OS-level sandbox or separate user identity for tool subprocesses.

A workspace can also grant its own additional directories without exposing them to every other workspace:

```bash
chatgpt-mcp workspace access add ws_... /path/to/build-cache
chatgpt-mcp workspace access list ws_...
chatgpt-mcp workspace access remove ws_... /path/to/build-cache
```

The effective filesystem scope for an Agent is the workspace root plus global `permissions.allow_dirs` plus that workspace's `allow_dirs`. Filesystem reads/writes, shell mutation validation, Git/process working directories, and rewind validation use the same canonical root set. Symlink escapes remain denied. Filesystem mutations in allowed directories still create rewind checkpoints, and revoking a directory prevents old checkpoints from restoring files back into the revoked path. Agents can inspect effective roots with `list_allowed_directories` or `agent_status`, but no MCP tool can grant new directories to itself.

## CLI

```text
chatgpt-mcp
├── up
├── down
├── serve
├── init
├── uninit
├── status
├── logs
│   ├── follow
│   ├── path
│   └── clear
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

The same structured runtime events are persisted before terminal visibility filtering, so a normally started service can later be inspected at default, verbose, or debug detail without having needed `--debug` at startup:

```bash
chatgpt-mcp logs
chatgpt-mcp logs --verbose
chatgpt-mcp logs --debug
chatgpt-mcp logs --log-format=json
chatgpt-mcp logs -n 200
chatgpt-mcp logs -f
chatgpt-mcp logs follow
```

History is available even after the runtime stops. `-f` first replays matching history and then follows the authenticated loopback runtime event stream without polling the journal. Filters operate on structured event fields before rendering:

```bash
chatgpt-mcp logs --since 30m
chatgpt-mcp logs --level warn
chatgpt-mcp logs --component SERVER,TUNNEL
chatgpt-mcp logs --workspace ws_...
chatgpt-mcp logs --workspace /path/to/workspace
chatgpt-mcp logs --tool run_command --status error
chatgpt-mcp logs --source tunnel
chatgpt-mcp logs --event 'tool.call.*'
chatgpt-mcp logs --grep timeout
```

`logs path` prints the selected config root's journal path. `logs clear --force` clears the current and rotated journal through the runtime control channel when the server is running and directly when it is stopped. Reading/following logs is read-only in MCP tool execution context; clearing logs is not.

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
CHATGPT_MCP_CONFIG_DIR="$(mktemp -d)" go test ./...
go vet ./...
go build -trimpath ./
```

Run the native release smoke against a built binary:

```bash
go build -trimpath -o chatgpt-mcp ./
node scripts/smoke-release.mjs ./chatgpt-mcp
```

The smoke uses an isolated temporary home plus an explicit non-default `--config-dir`, and verifies init/config/status, live config reload with listener rebind/rollback, foreground and managed runtime metadata, persistent log replay/filter/follow/clear behavior, HTTP health, MCP discovery, tool listing, modern error behavior, shutdown, and uninit without touching the normal user config root. OS service definitions are covered by platform-specific tests while the portable smoke uses the hidden managed runtime entrypoint so CI does not require systemd, launchd, or Task Scheduler. Native Go test jobs also set `CHATGPT_MCP_CONFIG_DIR` to a runner-temporary non-default root, while runtime-heavy package test harnesses create their own isolated config roots.

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
