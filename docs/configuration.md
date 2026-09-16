# Configuration

`chatgpt-mcp` keeps persistent configuration and runtime state under one selected config root. Use this guide for the configuration model and common operations; use `cgm config explain` for the exhaustive schema of the installed version.

## Config root

Default:

```text
~/.config/chatgpt-mcp/
```

Select another root per command:

```bash
cgm --config-dir /path/to/instance status
```

or by environment:

```bash
export CHATGPT_MCP_CONFIG_DIR=/path/to/instance
```

Precedence is:

```text
--config-dir
> CHATGPT_MCP_CONFIG_DIR
> default user config root
```

A config root owns that instance's configuration, workspaces, secrets, upstream state, logs, shell/runtime state, memory, checkpoints, and runtime-control metadata. Use isolated roots for tests or parallel instances.

## Inspect configuration

```bash
cgm config get
cgm config list
cgm config get server
cgm config get admin.enabled
```

Structured display is available where supported:

```bash
cgm config list --json
cgm config list --yaml
cgm config list --toml
```

Sensitive fields are redacted.

## Explain the schema

`cgm config explain` is the authoritative configuration reference for the installed binary:

```bash
cgm config explain
cgm config explain server
cgm config explain server.expose.mode
cgm config explain shell.path
cgm config explain shell.path --json
```

A branch explains a subtree; a leaf reports its type, built-in default, editability, valid values, guidance, and related settings where applicable.

The public docs intentionally do not duplicate every schema field, because that inventory would drift from the binary.

## Set values

```bash
cgm config set server.enabled false
cgm config set server.port 41021
cgm config set admin.enabled true
```

Values are parsed according to the schema and validated before persistence. `key=value` syntax is also accepted by the CLI.

At least one MCP transport must remain enabled: direct MCP HTTP (`server.enabled`) or at least one enabled local Secure MCP Tunnel instance. Tunnel instances are managed through `cgm tunnel ...` rather than scalar `config set` fields.

## Desktop notifications

Pending control approvals can raise a best-effort desktop notification when no TUI reviewer is open. Notification failure never blocks, approves, or denies the request.

```bash
cgm config set notifications.enabled true
cgm config set notifications.approvals true
cgm config set notifications.when_tui_inactive true
cgm config set notifications.open_action auto
cgm config explain notifications
```

Platform behavior:

- **Linux desktop** uses the session notification bus (`gdbus` or `notify-send`) when `DBUS_SESSION_BUS_ADDRESS` or `XDG_RUNTIME_DIR` is present. Headless and SSH sessions without a session bus stay silent.
- **Windows** uses a PowerShell balloon tip when `powershell.exe` or `pwsh.exe` is available.
- **macOS** uses `osascript` when present.
- **WSL** prefers the Windows host (`powershell.exe`) and does not require Linux D-Bus. Terminal launch, when later enabled, keeps the current WSL distro through Windows Terminal.

v1 notifications are passive: they do not approve/deny, and they do not yet click-open a terminal. Open the TUI Requests page or `cgm tui requests <request-id>` to review.

`cgm doctor --verbose` reports whether a desktop provider looks available; unavailability is non-fatal.

## Applying changes to a running runtime

Supported local config mutations are applied to the selected running runtime automatically. If the runtime is stopped, the persisted value is used on the next start.

Network-affecting changes such as listener ports or exposure are rebound transactionally. If the new listener cannot be opened, the working listener set is retained and the local mutation reports failure rather than silently leaving runtime and disk in different states.

Verify after meaningful access/network changes:

```bash
cgm config verify
cgm config verify --strict
```

## Storage format

JSON is the default structured format. YAML and TOML are also supported:

```bash
cgm init --json
cgm init --yaml
cgm init --toml
```

Convert an existing managed structured state tree:

```bash
cgm config convert json
cgm config convert yaml
cgm config convert toml
```

Conversion validates the managed state before activating the new representation.

## Secrets

Long-lived reversible credentials such as per-instance tunnel runtime keys, tunnel admin-profile keys, and sensitive upstream header/environment values are stored through the selected config root's managed secret store rather than as plaintext values in ordinary structured config.

MCP/Admin endpoint credentials are represented by one-way hashes where appropriate. Normal config/status output does not reveal managed secrets.

Migrate older plaintext credential state:

```bash
cgm config migrate
```

Encrypt legacy plaintext secret-store files:

```bash
cgm config migrate secrets
```

See [Security](security.md) for the storage and trust model.

## Authentication

MCP and Admin endpoint authentication are separate policies. Direct MCP HTTP authentication protects `/mcp` only; Secure MCP Tunnel uses separate credentials and is unaffected. Reuse the Direct MCP HTTP token when adding this MCP server to ChatGPT — you do not need a new token for each ChatGPT configuration.

```bash
cgm auth status
cgm auth mcp create
cgm auth admin create
cgm auth mcp enable
cgm auth admin enable
```

Direct authenticated HTTP clients use the credential expected by that endpoint/transport. The OpenAI tunnel runtime API key is different: it authenticates the tunnel client to OpenAI and is not a Direct MCP HTTP or Admin bearer token.

Generic protected `cgm mcp http` uses the same Direct MCP HTTP token as managed `/mcp`. Secure MCP Tunnel credentials are separate.

## Network exposure

The safest direct-listener posture is loopback-only:

```bash
cgm config set server.expose none
```

Other supported exposure modes can bind selected interfaces or broader addresses, but non-loopback direct HTTP changes the trust model and requires the appropriate authentication/insecure-HTTP acknowledgement.

For ChatGPT, prefer Secure MCP Tunnel instead of opening the MCP listener publicly. Add a management profile, then attach one or more managed tunnels:

```bash
cgm tunnel admin add personal --admin-key 'sk-admin-...' --organization-id org_...
cgm tunnel admin verify personal
cgm tunnel attach tunnel_... --admin personal --runtime-api-key 'sk-...'
```

Read [Security](security.md#network-exposure) before widening exposure.

## Workspace access

Register concrete project roots with:

```bash
cgm workspace register ~/projects/my-project
```

Workspace-owned identity, memory, checkpoints, and related state live in `<workspace>/.cgm`. The config root keeps only a path/id index plus global application state.

Workspace-specific extra roots:

```bash
cgm workspace access add ws_... /path/to/cache
```

Global extra roots:

```bash
cgm config set permissions.allow_dirs /path/one,/path/two
```

See [Workspaces](workspaces.md) for the canonical `ws_*` / `wsc_*` model and effective scope rules.

## Shell execution

Shell commands inherit the runtime process environment, with configured `shell.path` entries prepended to `PATH`.

`chatgpt-mcp` does not claim to provide a configurable kernel-level process sandbox. Workspace containment, protected control-plane state, and runtime approval/control-guard rules are application-level boundaries. Use an OS sandbox, container/VM, or separate operating-system identity when stronger isolation is required.

See [Security](security.md#shell-execution-boundary).

## Tunnel configuration

Tunnel state is a collection. Each local tunnel instance has its own ID/runtime key and may reference an admin profile used for management provenance. Admin profiles hold management credentials/scopes and are not runtime connections.

Conceptually the persisted model is:

```yaml
tunnel:
  instances:
    - id: tunnel_a
      enabled: true
      admin_profile_id: personal
    - id: tunnel_b
      enabled: true
  admins:
    - id: personal
      organization_id: org_...
```

Runtime/admin keys are stored separately in the managed secret store and are redacted from normal config/status output. Older scalar tunnel config is migrated to one collection instance plus a `default` admin profile when applicable.

Use:

```bash
cgm tunnel list
cgm tunnel status [tunnel_id]
cgm tunnel attach tunnel_... --admin personal --runtime-api-key 'sk-...'
cgm tunnel detach tunnel_...
cgm tunnel admin list
cgm tunnel managed list
```

See [OpenAI + ChatGPT](openai-chatgpt.md) for Platform and ChatGPT setup. Use `cgm tunnel --help` for the current local/managed tunnel command surface.

## Upstream MCP configuration

Manage upstream servers with:

```bash
cgm upstream --help
cgm upstream server --help
```

See [MCP and upstreams](mcp.md).

## Portable backup and transfer

Export the selected portable configuration/state plus managed reversible secrets:

```bash
cgm config export
```

Import it on another supported installation:

```bash
cgm config import
```

Both default to `chatgpt-mcp-config.cgm` in the current directory; provide an explicit path when needed.

The portable bundle intentionally excludes transient machine-owned state such as runtime control/PIDs, logs, service-manager definitions, shell session history, checkpoints, and update cache. Import requires the selected runtime to be stopped and protects existing state unless replacement is explicitly requested.

## Remove local config/state

```bash
cgm uninit
```

`uninit` removes the selected config/state root. It is different from uninstalling the binary. Stop the matching managed service first when appropriate.
