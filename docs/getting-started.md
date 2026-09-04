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

Direct bootstrap installers only download, verify, and extract a release; the binary owns the managed installation layout. If you downloaded a release archive manually, install it with:

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

## Update

Check without changing files:

```bash
cgm update check
```

Update a managed direct installation to the latest stable release:

```bash
cgm update
```

Install an exact release, including an intentional downgrade:

```bash
cgm update --version vX.Y.Z
```

If the selected config root has a running managed service, update switches the stable `current` target, restarts that service, and waits for runtime readiness. If the new runtime fails to become healthy, `chatgpt-mcp` restores the previous `current` target and metadata, then restarts the previous version.

Skip the managed-service restart when you intentionally want the running process to remain on the old binary until a later restart:

```bash
cgm update --no-restart
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

## Start the runtime

Foreground:

```bash
cgm serve
```

Default endpoints:

```text
MCP:   http://127.0.0.1:37421/mcp
Admin: http://127.0.0.1:37422/
```

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
