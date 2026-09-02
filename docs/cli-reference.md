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
├── auth
│   ├── mcp
│   ├── admin
│   └── status
├── config
│   ├── convert
│   ├── get
│   ├── list
│   ├── path
│   ├── preset
│   ├── reload
│   ├── set
│   └── verify
├── down
├── init
├── logs
│   ├── follow
│   ├── path
│   └── clear
├── mcp
│   └── server
├── serve
├── status
├── tunnel
│   ├── configure
│   ├── disable
│   ├── enable
│   ├── run
│   └── status
├── uninit
├── up
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

Migrate legacy plaintext credentials to the OS keyring:

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
