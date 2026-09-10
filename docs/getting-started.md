# Getting started

This guide gets `chatgpt-mcp` installed, initialized, and running locally. If your goal is specifically to connect ChatGPT through OpenAI Secure MCP Tunnel, continue with [OpenAI + ChatGPT setup](openai-chatgpt.md) after initialization.

## Install

### Linux / macOS

```bash
curl -fsSL get.mewis.me/chatgpt-mcp.sh | sh
```

### Windows PowerShell

```powershell
irm https://get.mewis.me/chatgpt-mcp.ps1 | iex
```

### Homebrew

```bash
brew tap mewisme/mew
brew install --cask chatgpt-mcp
```

### Scoop

```powershell
scoop bucket add mew https://github.com/mewisme/scoop-mew
scoop install mew/chatgpt-mcp
```

The installers expose both commands:

```text
chatgpt-mcp
cgm
```

The rest of this guide uses `cgm`.

Direct bootstrap installers download the release archive, verify its SHA-256 against `checksums.txt`, and (when available) verify that checksum file with Sigstore via `cosign` and `checksums.txt.sigstore.json`. They extract only the `chatgpt-mcp` binary (rejecting unsafe archive paths and symlinks). If cosign or the signature artifact is unavailable, set `INSTALL_ALLOW_CHECKSUM_ONLY=1` to proceed with a loud checksum-only warning; otherwise the installer fails. The binary owns the managed installation layout. If you downloaded a release archive manually, install it with:

```bash
./chatgpt-mcp install
```

Skip the short alias when needed:

```bash
./chatgpt-mcp install --no-alias
```

## Pin a release

Linux/macOS:

```bash
curl -fsSL get.mewis.me/chatgpt-mcp.sh | env CHATGPT_MCP_VERSION=vX.Y.Z sh
```

Windows:

```powershell
$env:CHATGPT_MCP_VERSION = 'vX.Y.Z'
irm https://get.mewis.me/chatgpt-mcp.ps1 | iex
```

The installers keep a stable launcher path so managed service definitions continue to work across upgrades.

Checksum-only bootstrap (when cosign or `checksums.txt.sigstore.json` is unavailable):

```bash
curl -fsSL get.mewis.me/chatgpt-mcp.sh | env INSTALL_ALLOW_CHECKSUM_ONLY=1 sh
```

```powershell
$env:INSTALL_ALLOW_CHECKSUM_ONLY = '1'
irm https://get.mewis.me/chatgpt-mcp.ps1 | iex
```

## Update

Check without changing files:

```bash
cgm upgrade check
```

Update a managed direct installation to the latest stable release:

```bash
cgm upgrade
```

Install an exact release, including an intentional downgrade:

```bash
cgm upgrade --version vX.Y.Z
```

If the selected config root has a running managed service, update switches the stable `current` target, restarts that service, and waits for full runtime readiness. When the Secure MCP Tunnel is enabled, that includes waiting for the tunnel to become ready instead of returning while it is still connecting. If the new runtime fails to become healthy, `chatgpt-mcp` restores the previous `current` target and metadata, then restarts the previous version.

Skip the managed-service restart when you intentionally want the running process to remain on the old binary until a later restart:

```bash
cgm upgrade --no-restart
```

A foreground `cgm serve` process is never killed by the updater; the files on disk are updated and that foreground process continues using its old in-memory binary until restarted manually.

Install ownership is preserved:

- managed direct install → built-in transactional self-update
- Homebrew → reports `brew upgrade --cask chatgpt-mcp`
- Scoop → reports `scoop update chatgpt-mcp`
- `go install` / development builds → built-in self-update is refused
- standalone release binary → run `chatgpt-mcp install` first to adopt the managed layout

Explicit update checks use the network. Normal commands do not; `cgm status` may surface fresh cached availability from `<install-root>/state/update.json`.

## Uninstall the binary

Linux/macOS:

```bash
curl -fsSL get.mewis.me/chatgpt-mcp.sh | sh -s -- --uninstall
```

Windows:

```powershell
& ([scriptblock]::Create((irm https://get.mewis.me/chatgpt-mcp.ps1))) -Uninstall
```

Binary uninstall and `cgm uninit` are different operations. `uninit` removes the selected `chatgpt-mcp` config/state root; the installer uninstall removes the installed command.

## Initialize

```bash
cgm init
```

JSON is the default storage format. YAML and TOML are also supported:

```bash
cgm init --json
cgm init --yaml
cgm init --toml
cgm init --format toml
```

Initialization creates the local configuration and authentication material under:

```text
~/.config/chatgpt-mcp/
```

on the selected user account.

## Isolated config roots

Use an isolated root for tests, experiments, or parallel instances:

```bash
cgm --config-dir ./.tmp/cgm-dev init
cgm --config-dir ./.tmp/cgm-dev serve
```

or:

```bash
export CHATGPT_MCP_CONFIG_DIR="$PWD/.tmp/cgm-dev"
cgm init
cgm serve
```

Precedence is:

```text
--config-dir
    >
CHATGPT_MCP_CONFIG_DIR
    >
default ~/.config/chatgpt-mcp
```

The selected root includes configuration, tunnel secrets, workspaces, OAuth/upstream state, shell state, memory, checkpoints, logs, and runtime control state.

## Register your first workspace

```bash
cgm workspace register ~/projects/my-project
```

Example output includes a stable ID such as:

```text
ws_...
```

Use the workspace ID in tool calls and workspace-specific access rules.

Inspect registered workspaces:

```bash
cgm workspace list
cgm workspace show ws_...
```

Grant one workspace access to an extra directory:

```bash
cgm workspace access add ws_... /path/to/build-cache
cgm workspace access list ws_...
```

See [Configuration](configuration.md) and [Security](security.md) before broadening filesystem scope.

## Open the interactive Command Center

For human-driven administration, launch the full-screen TUI explicitly:

```bash
cgm tui
```

You can also deep-link to a page or resource:

```bash
cgm tui workspace
cgm tui workspace ws_...
cgm tui mcp github
cgm tui logs
cgm tui config
```

`Ctrl+P` opens the Command Palette, `Ctrl+O` opens resource/page search, and `Alt+Left` / `Alt+Right` cycle the main pages. The TUI requires terminal stdin/stdout; use ordinary `cgm ...` commands and structured flags such as `--json` in scripts or pipelines.

See [TUI Command Center](tui.md) for the complete interaction model.

## Start the runtime

Foreground:

```bash
cgm serve
```

Default local endpoints:

```text
MCP:   http://127.0.0.1:37421/mcp
Admin: http://127.0.0.1:37422/
```

The MCP HTTP endpoint exists only while `server.enabled=true`. You may instead run tunnel-only with `server.enabled=false` and `tunnel.enabled=true`; both MCP transports cannot be disabled at the same time.

For a managed background runtime:

```bash
cgm up
```

Inspect it:

```bash
cgm status
cgm logs -f
```

Stop and remove only the managed service:

```bash
cgm down
```

`down` preserves configuration, workspaces, checkpoints, and runtime logs.

See [Runtime and services](runtime.md) for Linux/macOS `sudo` behavior, SSH/logout semantics, Windows Task Scheduler, persistent logs, and service lifecycle details.

## Connect ChatGPT

The recommended private path is OpenAI Secure MCP Tunnel. Continue with:

[Connect ChatGPT with OpenAI Secure MCP Tunnel →](openai-chatgpt.md)

## Build from source

Requirements:

- Go 1.27+
- Node.js 24+
- pnpm 11+

```bash
git clone https://github.com/mewisme/chatgpt-mcp.git
cd chatgpt-mcp
pnpm --dir web install
node scripts/install-local.mjs
```

Useful variants:

```bash
node scripts/install-local.mjs --no-deps
node scripts/install-local.mjs --from-dist
```

See [Development](development.md) for the complete verification flow.
