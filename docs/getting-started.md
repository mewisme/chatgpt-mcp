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
cmcp
```

The rest of this guide uses `cmcp`.

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

## Uninstall the binary

Linux/macOS:

```bash
curl -fsSL get.mewis.me/chatgpt-mcp.sh | sh -s -- --uninstall
```

Windows:

```powershell
& ([scriptblock]::Create((irm https://get.mewis.me/chatgpt-mcp.ps1))) -Uninstall
```

Binary uninstall and `cmcp uninit` are different operations. `uninit` removes the selected `chatgpt-mcp` config/state root; the installer uninstall removes the installed command.

## Initialize

```bash
cmcp init
```

JSON is the default storage format. YAML and TOML are also supported:

```bash
cmcp init --json
cmcp init --yaml
cmcp init --toml
cmcp init --format toml
```

Initialization creates the local configuration and authentication material under:

```text
~/.config/chatgpt-mcp/
```

on the selected user account.

## Isolated config roots

Use an isolated root for tests, experiments, or parallel instances:

```bash
cmcp --config-dir ./.tmp/cmcp-dev init
cmcp --config-dir ./.tmp/cmcp-dev serve
```

or:

```bash
export CHATGPT_MCP_CONFIG_DIR="$PWD/.tmp/cmcp-dev"
cmcp init
cmcp serve
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
cmcp workspace register ~/projects/my-project
```

Example output includes a stable ID such as:

```text
ws_...
```

Use the workspace ID in tool calls and workspace-specific access rules.

Inspect registered workspaces:

```bash
cmcp workspace list
cmcp workspace show ws_...
```

Grant one workspace access to an extra directory:

```bash
cmcp workspace access add ws_... /path/to/build-cache
cmcp workspace access list ws_...
```

See [Configuration](configuration.md) and [Security](security.md) before broadening filesystem scope.

## Start the runtime

Foreground:

```bash
cmcp serve
```

Default endpoints:

```text
MCP:   http://127.0.0.1:37421/mcp
Admin: http://127.0.0.1:37422/
```

For a managed background runtime:

```bash
cmcp up
```

Inspect it:

```bash
cmcp status
cmcp logs -f
```

Stop and remove only the managed service:

```bash
cmcp down
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
