# CLI reference

`chatgpt-mcp` and `cgm` are equivalent commands. This reference uses `cgm` for brevity.

Use the built-in help as the authoritative command surface:

```bash
cgm --help
cgm <command> --help
```

## Useful aliases

Only a small set of high-value aliases is provided:

| Full name | Alias |
| --- | --- |
| `config` | `cfg` |
| `workspace` | `ws` |
| `list` | `ls` |
| `status` | `st` |

Aliases compose with nested commands, for example `cgm cfg ls`, `cgm ws ls`, `cgm upstream server ls`, and `cgm tunnel st`.

## Shell completion

Cobra provides command, subcommand, flag, and dynamic argument completion for Bash, Zsh, Fish, and PowerShell:

```bash
# Bash
source <(cgm completion bash)

# Zsh
source <(cgm completion zsh)

# Fish
cgm completion fish | source
```

```powershell
cgm completion powershell | Out-String | Invoke-Expression
```

Each generated script registers both `chatgpt-mcp` and `cgm`, so completion keeps working regardless of which binary name or installed alias is used. Dynamic completion includes config keys and typed values, workspace IDs, upstream MCP IDs, recent runtime session IDs, and directory arguments where appropriate. For example, `cgm cfg set per<Tab>` completes `permissions.allow_dirs`, while `cgm cfg set auth.mcp_enabled <Tab>` offers `true` and `false`.

For source-tree development with direct `go run .` invocations, Bash and Zsh can opt into the Go wrapper hook:

```bash
source <(go run . completion bash --go-run)
# or, in Zsh:
source <(go run . completion zsh --go-run)
```

The hook only redirects completion when the command starts with `go run .`; otherwise it delegates to the previously registered Go completion function when one exists.

## Global flags

| Flag | Purpose |
| --- | --- |
| `--config-dir <path>` | select config/state root; overrides `CHATGPT_MCP_CONFIG_DIR` |
| `--verbose` | show additional operational context |
| `--debug` | show full diagnostic logging |
| `--log-format text\|json` | choose human CLI-first text or JSON event output |
| `--expose[=<value>]` | one-run network exposure override for commands that start the server |
| `-v`, `--version` | print binary version |

## Command tree

```text
chatgpt-mcp
├── alias
│   ├── install
│   ├── remove
│   └── status
├── auth
│   ├── mcp
│   ├── admin
│   └── status
├── completion
├── config
│   ├── convert
│   ├── export
│   ├── get
│   ├── import
│   ├── list
│   ├── migrate
│   │   └── secrets
│   ├── path
│   ├── reload
│   ├── set
│   └── verify
├── down
├── init
├── install
├── logs
│   ├── follow
│   ├── path
│   └── clear
├── mcp
│   ├── http
│   ├── stdio
│   └── server      # deprecated compatibility path
├── request
│   ├── approve
│   ├── create
│   │   └── dummy
│   ├── deny
│   ├── list
│   └── view
├── serve
├── status
├── tui
├── tunnel
│   ├── admin
│   ├── configure
│   ├── create
│   ├── delete
│   ├── disable
│   ├── enable
│   ├── get
│   ├── list
│   ├── run
│   ├── status
│   ├── sync
│   └── update
├── uninit
├── upstream
│   └── server
│       ├── add
│       ├── auth
│       ├── configure
│       ├── disable
│       ├── enable
│       ├── list
│       ├── remove
│       ├── show
│       ├── status
│       └── tools
├── up
├── update
│   └── check
├── version
└── workspace
    ├── access
    │   ├── add
    │   ├── list
    │   └── remove
    ├── list
    ├── relocate
    ├── register
    ├── show
    └── unregister
```

## Installation and updates

Install the current binary into the managed direct-install layout:

```bash
chatgpt-mcp install
chatgpt-mcp install --no-alias
```

Manage the short alias independently:

```bash
cgm alias install
cgm alias status
cgm alias remove
```

Check or apply updates:

```bash
cgm upgrade check
cgm upgrade
cgm upgrade --version vX.Y.Z
cgm upgrade --no-restart
```

`upgrade check` is read-only and always checks the latest release. Built-in mutation is only available for managed direct installs. Homebrew and Scoop installs report the owning package-manager upgrade command; Go/development installs refuse built-in self-update; standalone binaries must run `chatgpt-mcp install` first. `update` remains an alias for compatibility.

Direct updates download the expected platform archive and `checksums.txt`, verify SHA-256 before extraction/activation, preserve the current `cgm` alias state, and switch the stable `current` target transactionally. Exact `--version` allows an intentional downgrade.

When the selected config root has a managed runtime, `cgm upgrade` restarts it and waits for full readiness. If the Secure MCP Tunnel is enabled, readiness includes the tunnel reaching its ready state; connecting/reconnecting is not treated as success. Failure restores the previous install target and metadata and restarts the previous runtime. `--no-restart` leaves an existing process on the previous binary; foreground `serve` is also never killed by the updater.

`cgm status` never performs a network update check. It may show availability from the fresh install-global cache at `<install-root>/state/update.json`.

## Control approval requests

When an MCP tool hits an approvable control guard, the agent can create a short-lived human request with the `request_control_approval` MCP tool. Local operators inspect and resolve those requests through the running runtime:

```bash
cgm request list
cgm request view <request_id>
cgm request approve <request_id>
cgm request deny <request_id>
cgm request create dummy
```

Aliases include `req`, `ls`, `show`/`info`, `accept`/`allow`, and `reject`. Request IDs may be specified in full or by an unambiguous prefix. `approve` and `deny` accept `--reason`; list/view/resolve commands support `--json` where applicable.

Pending requests expire after 60 seconds. Approval does not grant a general CLI bypass: it authorizes one exact retry of the original MCP tool arguments. A mismatched retry is rejected without consuming the valid grant; a successful retry consumes it. `cgm request approve/deny` cannot be run by an MCP shell tool to self-approve its own request.

`cgm request create dummy` creates a short-lived pending request through the same runtime approval manager and event stream as production requests. It is intended for testing the request TUI and admin approval UI; its random dummy session cannot match a real MCP retry grant.

## TUI Command Center

`cgm tui` is the dedicated full-screen interactive application. Normal CLI commands remain the stable scriptable interface.

```bash
cgm tui
cgm tui workspace
cgm tui workspace ws_...
cgm tui mcp github
cgm tui logs
cgm tui config
```

The TUI requires terminal stdin/stdout. Its global navigation uses `Ctrl+P` for the Command Palette, `Ctrl+O` for Quick Open, `Alt+Left` / `Alt+Right` to cycle top-level pages, and `Esc` to close the current overlay or navigate back.

Use explicit `cgm tui` for interactive work and ordinary CLI/JSON output for automation. List commands do not auto-open a TUI and no longer expose per-command `--interactive` / `--no-interactive` flags.

See [TUI Command Center](tui.md) for Command Palette search, Quick Open, mouse behavior, deep links, forms, confirmations, and scripting guidance. See [Security](security.md#control-guard-approvals-and-self-grant-prevention) for approval challenge binding and one-shot capability semantics.

## Lifecycle

### Initialize

```bash
cgm init
cgm init --json
cgm init --yaml
cgm init --toml
```

### Foreground runtime

```bash
cgm serve
cgm serve --verbose
cgm serve --debug
cgm serve --expose=eth0
```

### Managed runtime

```bash
cgm up
cgm status
cgm down
```

Linux/macOS system scope:

```bash
cgm up --system
cgm down --system
```

When invoked from a normal user shell, `--system` automatically re-executes the stable absolute `cgm` launcher through `sudo`, so it does not depend on `sudo` including `~/.local/bin` in `secure_path`. For `go run . up|down|restart --system`, the transient Go build is first staged by the invoking user and that staged binary is passed to `sudo`; elevated service management therefore never creates a root-owned `runtime/bin/go-run` cache inside the user's config root. Running the absolute binary under `sudo` directly remains supported for compatibility.

See [Runtime and services](runtime.md).

## Logs

History:

```bash
cgm logs
cgm logs -n 200
cgm logs --verbose
cgm logs --debug
cgm logs --log-format=json
cgm logs --no-time
```

Runtime log replay is timestamped by default and separated by runtime session. Normal CLI command output remains timestamp-free. Use `--session <run_id-or-prefix>` to isolate one runtime process and `--no-time` to suppress replay timestamps.

Follow:

```bash
cgm logs -f
cgm logs follow
```

Filters:

```bash
cgm logs --since 30m
cgm logs --until 2026-08-31T12:00:00+07:00
cgm logs --session run_a1b2c3d4e5f6
cgm logs --level warn
cgm logs --component SERVER,TUNNEL
cgm logs --workspace ws_...
cgm logs --workspace ~/projects/my-project
cgm logs --tool run_command
cgm logs --status error
cgm logs --source tunnel
cgm logs --event 'tool.call.*'
cgm logs --grep timeout
```

Journal management:

```bash
cgm logs path
cgm logs clear --force
```

## Configuration

Inspect persisted values:

```bash
cgm config get
cgm config list
cgm config get admin.enabled
cgm config list admin
```

Explain schema keys and branches:

```bash
cgm config explain
cgm config explain shell
cgm config explain shell.path
cgm config explain server.expose.mode
cgm config explain shell.path --json
```

`config explain` is schema-driven and read-only. With no key it walks the full config schema; a branch such as `shell` returns that subtree; a leaf returns its description, details, type, built-in default, editability, valid enum values, guidance, and related keys when available. The reported default is the schema default, not the current persisted value. Legacy aliases are canonicalized before lookup, and sensitive fields expose metadata only, never secret values.

Set:

```bash
cgm config set server.enabled false
cgm config set server.port 41021
cgm config set admin.port 41022
cgm config set server.expose none
```

At least one MCP transport must remain enabled: `server.enabled` for direct MCP HTTP or `tunnel.enabled` for OpenAI Secure MCP Tunnel.

Successful config mutations automatically apply to a running process. If the runtime is stopped, they take effect on the next start.

Migrate legacy plaintext credentials to the per-config-root secret file store:

```bash
cgm config migrate
```

Encrypt plaintext files already in the per-config-root secret store (AES-256-GCM at rest):

```bash
cgm config migrate secrets
```

Verify:

```bash
cgm config verify
cgm config verify --strict
cgm config validate
```

Convert:

```bash
cgm config convert json
cgm config convert yaml
cgm config convert toml
cgm config transform toml
```

Portable backup/migration:

```bash
cgm config export
cgm config import
```

Both commands default to `chatgpt-mcp-config.cgm` in the current directory. Pass an explicit file only when a custom path/name is needed, for example `cgm config export laptop.cgm` and `cgm config import laptop.cgm`.

`config export` creates one sealed bundle containing portable persistent config/state plus all currently managed reversible secrets. `config import` restores that bundle on Linux, macOS, or Windows and rebuilds the destination secret store instead of copying source secret files. Existing config/state requires `--force` on import; an existing bundle requires `--force` on export. Import requires the selected runtime to be stopped.

Machine-local filesystem paths are normalized during import. Home-relative paths are mapped to the destination user's home when the corresponding directory exists; unavailable paths and workspaces are skipped. Runtime control state, logs, managed-service environment snapshots, instance identity, shell session state, checkpoints, update cache, and raw secret-store files are intentionally not migrated.

Structured display:

```bash
cgm config list --json
cgm config list --yaml
cgm config list --toml
```

## Authentication

```bash
cgm auth status
cgm auth mcp create
cgm auth admin create
cgm auth mcp enable
cgm auth mcp disable
cgm auth admin enable
cgm auth admin disable
```

`cgm mcp stdio` does not use OAuth transport authentication. `cgm mcp http` uses OAuth as the canonical protected transport and keeps the existing static MCP bearer only as a compatibility path controlled by `auth.mcp_legacy_bearer`.

```bash
cgm config set auth.mcp_legacy_bearer false
```

Rotating the MCP credential invalidates OAuth codes/tokens issued under the previous credential generation.

Use subcommand help for enable/disable/rotation options exposed by the current binary:

```bash
cgm auth mcp --help
cgm auth admin --help
```

## Workspaces

Register:

```bash
cgm workspace register ~/projects/my-project
```

Inspect:

```bash
cgm workspace list
cgm workspace show ws_...
```

If the project directory has already been renamed or moved, rebind the existing workspace instead of registering the destination as a new workspace:

```bash
cgm workspace relocate ws_... /new/path/to/project
```

`relocate` does not move project files. It updates the registered canonical root after the filesystem move, derives the new path-based workspace ID, retains the previous ID as a legacy alias, migrates workspace-scoped persistent state, rewrites state paths rooted under the old project directory, preserves container membership, and synchronizes a running runtime before returning. Workspace-specific extra roots that were inside the old root are rebased to the new root; unrelated external access roots are left unchanged. Managed background processes that were already started remain addressable through the relocated workspace while the current runtime is alive.

Manage logical workspace containers:

```bash
cgm workspace container list
cgm workspace container create "Backend projects"
cgm workspace container show wsc_...
cgm workspace container rename wsc_... "Services"
cgm workspace container add wsc_... ws_... [ws_...]
cgm workspace container remove wsc_... ws_... [ws_...]
cgm workspace container delete wsc_...
```

Container IDs use the `wsc_` prefix. Containers group registered workspaces without merging filesystem scope, project context, shell/REPL state, memory, or checkpoints.

Agent-facing MCP tools expose containers separately:

```text
workspace_container_list()
workspace_container_status(container_id="wsc_...")
workspace_container_context(container_id="wsc_...")
```

`wsc_*` is orchestration-only. Filesystem, Git, shell, memory, rule, checkpoint, and `project_context` calls still require one concrete member `ws_*` as `workspace_id`. Passing an existing container ID as `workspace_id` fails with guidance to resolve the container and choose a member; cgm never fans an operation out or silently selects the first member.

When the runtime is already running, every successful CLI workspace-registry mutation synchronously reloads runtime state before returning. This covers workspace register/relocate/unregister, access add/remove, container create/rename/delete, and membership add/remove. The next MCP read therefore sees the change without restarting the runtime or reconnecting the MCP session. If runtime synchronization fails, the CLI reports the failure even though the registry mutation may already have been persisted.

Remove the registry handle without deleting project files:

```bash
cgm workspace unregister ws_...
```

Additional workspace roots:

```bash
cgm workspace access add ws_... /path/to/cache
cgm workspace access list ws_...
cgm workspace access remove ws_... /path/to/cache
```

## OpenAI Secure MCP Tunnel

Configure:

```bash
cgm tunnel configure \
  --enabled \
  --id tunnel_... \
  --api-key 'sk-...'
```

Optional flags:

```text
--control-plane-base-url <url>
--organization-id <org_...>
```

Lifecycle:

```bash
cgm tunnel status
cgm tunnel enable
cgm tunnel disable
cgm tunnel run
```

See [OpenAI + ChatGPT setup](openai-chatgpt.md) for Platform/ChatGPT configuration.

## Upstream MCP servers

```bash
cgm upstream --help
cgm upstream server --help
cgm upstream server list
cgm upstream server show <id>
cgm upstream server status <id>
cgm upstream server tools <id>
cgm upstream server auth --help
```

Use the server subcommands to add, inspect, update, or remove upstream MCP definitions supported by the current binary.

`cgm mcp server ...` is retained temporarily as a deprecated compatibility path. New scripts and documentation should use `cgm upstream server ...`.

## Generic MCP clients

Local stdio:

```bash
cgm mcp stdio
cgm mcp stdio --workspace ~/projects/my-project
```

Cursor project configuration can use `--workspace ${workspaceFolder}` after that project has already been registered with `cgm workspace register`.

Standalone Streamable HTTP with legacy SSE fallback:

```bash
cgm mcp http
cgm mcp http --workspace ws_...
cgm mcp http --no-sse
```

The standalone HTTP mode is MCP-only and loopback-only in the current implementation. It does not start the Admin server or OpenAI Secure MCP Tunnel. Its primary endpoint is `/mcp`; legacy SSE compatibility is exposed at `/mcp/sse` unless disabled.

See [MCP and upstreams](mcp.md).

## Status

```bash
cgm status
```

Status is the main read-only overview for:

- config root
- runtime state
- foreground/managed service state
- service scope/backend/ID
- runtime session ID
- PID/start information
- MCP HTTP enabled/disabled state and endpoint when enabled
- tunnel enabled/configured/live state
- registered workspaces
- upstream servers
- cached update availability when a fresh install-global cache exists

`status` does not perform a network update check; use `cgm upgrade check` for an explicit fresh query.

## Isolated instances

One command:

```bash
cgm --config-dir /tmp/cgm-test status
```

Environment:

```bash
CHATGPT_MCP_CONFIG_DIR=/tmp/cgm-test cgm status
```

Always use an isolated config root when running destructive test/dev flows such as `init`, `uninit`, `config set`, workspace registration, or tunnel configuration.

## CLI logging modes

Default:

```bash
cgm status
```

Operational context:

```bash
cgm status --verbose
```

Full diagnostics:

```bash
cgm status --debug
```

Machine-readable:

```bash
cgm status --log-format=json
```

Visibility flags also apply when replaying persistent runtime logs.
