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

Aliases compose with nested commands, for example `cgm cfg ls`, `cgm ws ls`, `cgm mcp server ls`, and `cgm tunnel st`.

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

Dynamic completion includes config keys and typed values, preset names, workspace IDs, upstream MCP IDs, recent runtime session IDs, and directory arguments where appropriate. For example, `cgm cfg set per<Tab>` completes `permissions.allow_dirs`, while `cgm cfg set auth.mcp_enabled <Tab>` offers `true` and `false`.

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
├── config
│   ├── convert
│   ├── export
│   ├── get
│   ├── import
│   ├── list
│   ├── migrate
│   ├── path
│   ├── preset
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
│   └── server
├── request
│   ├── approve
│   ├── create
│   │   └── dummy
│   ├── deny
│   ├── list
│   └── view
├── serve
├── status
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
cgm update check
cgm update
cgm update --version vX.Y.Z
cgm update --no-restart
```

`update check` is read-only and always checks the latest release. Built-in mutation is only available for managed direct installs. Homebrew and Scoop installs report the owning package-manager upgrade command; Go/development installs refuse built-in self-update; standalone binaries must run `chatgpt-mcp install` first.

Direct updates download the expected platform archive and `checksums.txt`, verify SHA-256 before extraction/activation, preserve the current `cgm` alias state, and switch the stable `current` target transactionally. Exact `--version` allows an intentional downgrade.

When the selected config root has a managed runtime, `cgm update` restarts it and waits for readiness. Failure restores the previous install target and metadata and restarts the previous runtime. `--no-restart` leaves an existing process on the previous binary; foreground `serve` is also never killed by the updater.

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

## Interactive TUI commands

The following commands support interactive mode. On a terminal they open the TUI automatically unless `--no-interactive` is supplied.

| Command | Interactive detail |
| --- | --- |
| `cgm request list` | pending approval inbox; detail modal with `Overview`, `Arguments`, and `Guard` tabs plus interactive Allow/Deny actions |
| `cgm workspace list` | workspace browser with workspace action menus, container create/add/remove dialogs, multi-select membership, confirmations, details, and copy-ID support |
| `cgm mcp server list` | upstream server browser; detail tabs group `Overview`, `Connection`, and `Tools` |
| `cgm mcp server list --refresh` | refreshed health browser; detail tabs group `Overview`, `Tools`, and `Error` |
| `cgm tunnel list` | managed tunnel browser; detail tabs group `Overview` and `Scope` |

Mode flags are consistent across these commands: `--interactive` forces the TUI and requires terminal stdin/stdout, `--no-interactive` forces deterministic text/legacy output, and `--json` suppresses the TUI for machine-readable output. For compatibility, non-interactive `cgm tunnel list` remains JSON by default.

Common list controls are `j/k` or arrows to move, `/` to filter, `enter`/`v` to open details, `r` to refresh when available, `?` for full help, and `q` to quit. In tabbed detail dialogs, `←/→` or `h/l` switch tabs and `j/k` or `↑/↓` scroll the active tab.

The request detail dialog keeps action focus separate from tab navigation: `Tab`/`Shift+Tab` switch between Allow and Deny, `Enter` activates the focused action, and `a`/`d` remain direct shortcuts. Resolution opens a second confirmation dialog; that dialog defaults to Cancel, uses `←/→`, `h/l`, or `Tab` to change focus, and `Enter` to choose the focused button. `y` confirms directly while `n`, `Esc`, or `q` cancel.

See [Security](security.md#control-guard-approvals-and-self-grant-prevention) for challenge binding and one-shot capability semantics.

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

When invoked from a normal user shell, `--system` automatically re-executes the stable absolute `cgm` launcher through `sudo`, so it does not depend on `sudo` including `~/.local/bin` in `secure_path`. Running the absolute binary under `sudo` directly remains supported for compatibility.

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

Inspect:

```bash
cgm config get
cgm config list
cgm config get admin.enabled
cgm config list admin
```

Set:

```bash
cgm config set server.port 41021
cgm config set admin.port 41022
cgm config set server.expose none
```

Apply to a running process:

```bash
cgm config reload
```

Migrate legacy plaintext credentials to the per-config-root secret file store:

```bash
cgm config migrate
```

Verify:

```bash
cgm config verify
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
cgm mcp --help
cgm mcp server --help
cgm mcp server list
cgm mcp server show <id>
cgm mcp server status <id>
cgm mcp server tools <id>
cgm mcp server auth --help
```

Use the server subcommands to add, inspect, update, or remove upstream MCP definitions supported by the current binary.

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
- tunnel enabled/configured/live state
- registered workspaces
- upstream servers
- cached update availability when a fresh install-global cache exists

`status` does not perform a network update check; use `cgm update check` for an explicit fresh query.

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
