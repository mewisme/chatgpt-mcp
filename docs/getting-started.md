# Getting started

This is the default `chatgpt-mcp` path: install one local runtime, register the projects ChatGPT may use, attach one or more OpenAI Secure MCP Tunnels, and keep the shared runtime running as a managed service.

```text
install → init → register workspace → add admin profile → attach tunnel → cgm up → connect ChatGPT
```

## Requirements

- Linux, macOS, or Windows on `amd64` or `arm64`.
- A normal user account. The MCP runtime should not run as root.
- For ChatGPT: an OpenAI Secure MCP Tunnel, a restricted runtime API key with **Tunnels Read + Use**, and an admin credential/scope when using `cgm` to discover/manage/attach Platform tunnels.
- Outbound HTTPS to OpenAI. The default ChatGPT setup does **not** require a public inbound MCP port.

Docker is not required. Git is optional unless you want to use Git tools.

## 1. Install

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

The installed commands are `chatgpt-mcp` and its shorter alias, `cgm`.

## 2. Initialize

```bash
cgm init
```

The default config/state root is:

```text
~/.config/chatgpt-mcp/
```

For isolated instances, tests, or development runs, select another root with `--config-dir` or `CHATGPT_MCP_CONFIG_DIR`. See [Configuration](configuration.md#config-root).

## 3. Register a workspace

Register only project roots you want ChatGPT to reach:

```bash
cgm workspace register ~/projects/my-project
```

The command returns a stable `ws_*` workspace ID. Filesystem, shell, Git, process, context, memory, rules, skills, and checkpoint operations use explicit workspace targets.

```bash
cgm workspace list
```

Read [Workspaces](workspaces.md) before adding extra filesystem roots or using workspace containers.

## 4. Configure OpenAI Secure MCP Tunnel

Create a tunnel in OpenAI Platform and a restricted runtime API key with **Tunnels Read + Use**. Add an admin profile for the Platform scope, verify it, then attach the tunnel locally:

```bash
cgm tunnel admin add personal --admin-key 'sk-admin-...' --organization-id org_...
cgm tunnel admin verify personal
cgm tunnel attach tunnel_... --admin personal --runtime-api-key 'sk-...'
```

If the admin profile is allowed to create a restricted runtime key, `--auto-runtime-key` can replace `--runtime-api-key`; use `--project-id` when project selection is ambiguous.

Check the local configuration:

```bash
cgm tunnel list
cgm tunnel status tunnel_...
```

The tunnel ID is an identifier. The runtime API key is a secret used only to authenticate that tunnel client; the admin key is a separate management credential and is never substituted for the runtime key. Attaching another tunnel adds another ingress to the same runtime instead of replacing the first one.

For tunnel creation, associations, permissions, Developer Mode, and ChatGPT app setup, follow [OpenAI + ChatGPT](openai-chatgpt.md).

## 5. Start the runtime

For normal use, start the managed background runtime:

```bash
cgm up
```

Inspect it:

```bash
cgm status
cgm tunnel status
```

For one-off foreground testing, use:

```bash
cgm serve
```

`serve` stays attached to the current terminal. On a remote server, prefer a managed service instead of relying on an SSH session. See [Runtime and operations](runtime.md).

## 6. Connect ChatGPT

In ChatGPT:

1. Enable Developer Mode if required for your workspace/account.
2. Create a custom app.
3. Choose **Tunnel** as the connection type.
4. Select the `tunnel_...` attached locally for that app.
5. Run **Scan Tools**.
6. Review the discovered tools and create/enable the app.

Then test with a read-only action such as listing registered workspaces or reading runtime status.

## 7. Verify and operate

Useful commands:

```bash
cgm status
cgm tunnel status
cgm logs -f
cgm config verify
```

For interactive operation:

```bash
cgm tui
```

## Next steps

- [OpenAI + ChatGPT](openai-chatgpt.md) — complete tunnel/app setup
- [Workspaces](workspaces.md) — scope, extra roots, relocation, containers
- [Runtime and operations](runtime.md) — services, logs, updates
- [TUI Command Center](tui.md) — interactive operation
- [Security](security.md) — trust boundaries and recommended posture
- [Configuration](configuration.md) — config roots, auth, exposure, storage
- [MCP and upstreams](mcp.md) — generic MCP clients and upstream servers

## Advanced installation and development

The built-in updater, exact version selection, package-manager ownership, source builds, release verification, and CI workflows are intentionally kept out of this happy path. See [Runtime and operations](runtime.md#updates) and [Development](development.md).
