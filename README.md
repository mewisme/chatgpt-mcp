<div align="center">

# chatgpt-mcp

**A secure, workspace-bound bridge between ChatGPT and your machine.**

Single Go binary · MCP `2026-07-28` · OpenAI Secure MCP Tunnel · Managed services · Embedded admin UI

[![Latest Release](https://img.shields.io/github/v/release/mewisme/chatgpt-mcp?display_name=tag&sort=semver&style=flat-square)](https://github.com/mewisme/chatgpt-mcp/releases/latest)
[![CI](https://img.shields.io/github/actions/workflow/status/mewisme/chatgpt-mcp/ci.yml?branch=main&label=CI&style=flat-square)](https://github.com/mewisme/chatgpt-mcp/actions/workflows/ci.yml)
[![MCP](https://img.shields.io/badge/MCP-2026--07--28-111111?style=flat-square)](docs/mcp.md)
[![OpenAI](https://img.shields.io/badge/OpenAI-Secure%20MCP%20Tunnel-000000?style=flat-square&logo=openai)](docs/openai-chatgpt.md)
[![Go](https://img.shields.io/github/go-mod/go-version/mewisme/chatgpt-mcp?style=flat-square&logo=go)](go.mod)
[![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20Windows-555?style=flat-square)](#installation)
[![License](https://img.shields.io/github/license/mewisme/chatgpt-mcp?style=flat-square)](LICENSE)

[Getting started](docs/getting-started.md) · [Connect ChatGPT](docs/openai-chatgpt.md) · [CLI reference](docs/cli-reference.md) · [Security](docs/security.md) · [Troubleshooting](docs/troubleshooting.md)

</div>

---

`chatgpt-mcp` gives ChatGPT controlled access to local workspaces, shell/Git operations, upstream MCP servers, runtime logs, and administration without requiring you to expose your machine directly to the public internet.

It includes OpenAI Secure MCP Tunnel support directly in the binary, so the common private setup is simply:

```text
ChatGPT
   │
   │ OpenAI-hosted Secure MCP Tunnel
   ▼
chatgpt-mcp
   ├─ workspace-bound filesystem / shell / Git tools
   ├─ upstream MCP aggregation
   ├─ runtime journal + live logs
   └─ embedded admin dashboard
```

## Highlights

- Stateless MCP `2026-07-28` HTTP runtime at `/mcp`
- Builtin OpenAI Secure MCP Tunnel client with supervised reconnects
- Workspace-bound filesystem, shell, Git, rules, skills, checkpoints, and utilities
- Dynamic upstream MCP aggregation with OAuth and MRTR relay
- Managed background runtime via systemd, launchd, or Task Scheduler
- Persistent structured logs with session boundaries, replay timestamps, filters, `--verbose`, `--debug`, JSON, and live follow
- Live configuration reload with transactional listener rebind and rollback
- Embedded React admin dashboard
- Separate MCP/admin authentication and explicit network exposure controls
- Single-binary releases for Linux, macOS, and Windows on amd64/arm64

## Installation

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

Both `chatgpt-mcp` and the shorter `cmcp` alias are installed. The examples below use `cmcp`.

See [Getting started](docs/getting-started.md) for source builds, version pinning, uninstall, and platform details.

## 5-minute quick start

### 1. Initialize

```bash
cmcp init
```

The default config/state root is:

```text
~/.config/chatgpt-mcp/
```

Use `--config-dir` or `CHATGPT_MCP_CONFIG_DIR` for isolated instances.

### 2. Register a workspace

```bash
cmcp workspace register ~/projects/my-project
```

The returned `workspace_id` is the stable handle used by workspace-bound tools.

### 3. Connect OpenAI Secure MCP Tunnel

Create a tunnel and a restricted runtime API key in OpenAI Platform, then configure them locally:

```bash
cmcp tunnel configure \
  --enabled \
  --id tunnel_... \
  --api-key 'sk-...'
```

The runtime key should have **Tunnels Read + Use**. Tunnel creation/editing requires **Tunnels Read + Manage**. Associate the tunnel with the ChatGPT workspace that should be able to discover it.

For the complete OpenAI flow — tunnel ID, runtime key, permissions, Developer Mode, creating the ChatGPT app, Scan Tools, and verification — follow [Connect ChatGPT with OpenAI Secure MCP Tunnel](docs/openai-chatgpt.md).

### 4. Start the runtime

Foreground:

```bash
cmcp serve
```

Managed background service:

```bash
cmcp up
```

`up` reports the managed scope/backend, runtime session, PID/endpoints, and whether the OpenAI tunnel is enabled, configured, and currently connected/connecting.

On Linux/macOS, `cmcp up --system` installs a machine-level service and automatically elevates through `sudo` when the current process is user-scoped; the MCP process itself still runs as the invoking user. On Windows, `cmcp up` always uses a per-user Scheduled Task.

### 5. Verify

```bash
cmcp status
cmcp tunnel status
cmcp logs -f
```

Then create or enable the developer-mode app in ChatGPT and select the same tunnel.

## Common commands

| Goal | Command |
| --- | --- |
| Initialize | `cmcp init` |
| Start foreground | `cmcp serve` |
| Start managed service | `cmcp up` |
| Stop/remove managed service | `cmcp down` |
| Inspect runtime | `cmcp status` |
| Follow logs | `cmcp logs -f` |
| Full diagnostic logs | `cmcp logs --debug -f` |
| Verify config/state | `cmcp config verify` |
| Reload persisted config | `cmcp config reload` |
| Register workspace | `cmcp workspace register <path>` |
| Add workspace access | `cmcp workspace access add <workspace_id> <path>` |
| Inspect tunnel | `cmcp tunnel status` |
| Manage upstream MCPs | `cmcp mcp --help` |
| Rotate MCP/admin credentials | `cmcp auth --help` |

`--verbose`, `--debug`, and `--log-format=json` are global flags. Use `cmcp <command> --help` for the live command surface.

## Runtime endpoints

Defaults:

```text
MCP:   http://127.0.0.1:37421/mcp
Admin: http://127.0.0.1:37422/
```

The default exposure mode is loopback-only. Network exposure, authentication, config formats, reload semantics, and isolated config roots are documented in [Configuration](docs/configuration.md).

## Documentation

| I want to… | Read |
| --- | --- |
| Install and get a local runtime running | [Getting started](docs/getting-started.md) |
| Connect ChatGPT through OpenAI Secure MCP Tunnel | [OpenAI + ChatGPT setup](docs/openai-chatgpt.md) |
| Understand `serve`, `up`, `down`, services, status, and logs | [Runtime and services](docs/runtime.md) |
| Configure auth, exposure, formats, reload, and workspace access | [Configuration](docs/configuration.md) |
| Find commands and useful flag combinations | [CLI reference](docs/cli-reference.md) |
| Understand MCP protocol behavior and upstream aggregation | [MCP and upstreams](docs/mcp.md) |
| Understand trust boundaries and security controls | [Security](docs/security.md) |
| Build, test, run CI, and prepare releases | [Development](docs/development.md) |
| Diagnose tunnel, service, config, or port failures | [Troubleshooting](docs/troubleshooting.md) |

The full documentation index lives in [`docs/README.md`](docs/README.md).

## Security model

`chatgpt-mcp` is intentionally workspace-bound. Filesystem/shell/Git mutations are constrained to the registered workspace plus explicitly allowed directories, symlink escapes are rejected, and MCP tool execution cannot use the builtin shell to grant itself new control-plane permissions.

The OpenAI tunnel runtime key is stored separately from the main config and should be treated as a secret. A tunnel ID is an identifier, not a secret. Do not use a Platform Admin API key as the long-lived tunnel runtime key.

Read [Security](docs/security.md) before widening network exposure or granting additional filesystem roots.

## Development

Requirements for source builds:

- Go 1.27+
- Node.js 24+
- pnpm 11+

```bash
pnpm --dir web install
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web typecheck
pnpm --dir web build
node scripts/prepare-web-embed.mjs
CHATGPT_MCP_CONFIG_DIR="$(mktemp -d)" go test ./...
go vet ./...
go build -trimpath ./
```

See [Development](docs/development.md) for the release smoke, cross-platform matrix, and the repository rule that tests must never mutate the real default config root.

## License

MIT License. Copyright (c) 2026 Mew.
