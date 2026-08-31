# CLI reference

`chatgpt-mcp` and `cmcp` are equivalent commands. This reference uses `cmcp` for brevity.

Use the built-in help as the authoritative command surface:

```bash
cmcp --help
cmcp <command> --help
```

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
cmcp init
cmcp init --json
cmcp init --yaml
cmcp init --toml
```

### Foreground runtime

```bash
cmcp serve
cmcp serve --verbose
cmcp serve --debug
cmcp serve --expose=eth0
```

### Managed runtime

```bash
cmcp up
cmcp status
cmcp down
```

Linux/macOS system scope:

```bash
cmcp up --system
cmcp down --system
```

When invoked from a normal user shell, `--system` automatically re-executes the stable absolute `cmcp` launcher through `sudo`, so it does not depend on `sudo` including `~/.local/bin` in `secure_path`. Running the absolute binary under `sudo` directly remains supported for compatibility.

See [Runtime and services](runtime.md).

## Logs

History:

```bash
cmcp logs
cmcp logs -n 200
cmcp logs --verbose
cmcp logs --debug
cmcp logs --log-format=json
```

Follow:

```bash
cmcp logs -f
cmcp logs follow
```

Filters:

```bash
cmcp logs --since 30m
cmcp logs --until 2026-08-31T12:00:00+07:00
cmcp logs --level warn
cmcp logs --component SERVER,TUNNEL
cmcp logs --workspace ws_...
cmcp logs --workspace ~/projects/my-project
cmcp logs --tool run_command
cmcp logs --status error
cmcp logs --source tunnel
cmcp logs --event 'tool.call.*'
cmcp logs --grep timeout
```

Journal management:

```bash
cmcp logs path
cmcp logs clear --force
```

## Configuration

Inspect:

```bash
cmcp config get
cmcp config list
cmcp config get admin.enabled
cmcp config list admin
```

Set:

```bash
cmcp config set server.port 41021
cmcp config set admin.port 41022
cmcp config set server.expose none
```

Apply to a running process:

```bash
cmcp config reload
```

Verify:

```bash
cmcp config verify
cmcp config validate
```

Convert:

```bash
cmcp config convert json
cmcp config convert yaml
cmcp config convert toml
cmcp config transform toml
```

Structured display:

```bash
cmcp config list --json
cmcp config list --yaml
cmcp config list --toml
```

## Authentication

```bash
cmcp auth status
cmcp auth mcp create
cmcp auth admin create
cmcp auth mcp enable
cmcp auth mcp disable
cmcp auth admin enable
cmcp auth admin disable
```

Use subcommand help for enable/disable/rotation options exposed by the current binary:

```bash
cmcp auth mcp --help
cmcp auth admin --help
```

## Workspaces

Register:

```bash
cmcp workspace register ~/projects/my-project
```

Inspect:

```bash
cmcp workspace list
cmcp workspace show ws_...
```

Remove the registry handle without deleting project files:

```bash
cmcp workspace unregister ws_...
```

Additional workspace roots:

```bash
cmcp workspace access add ws_... /path/to/cache
cmcp workspace access list ws_...
cmcp workspace access remove ws_... /path/to/cache
```

## OpenAI Secure MCP Tunnel

Configure:

```bash
cmcp tunnel configure \
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
cmcp tunnel status
cmcp tunnel enable
cmcp tunnel disable
cmcp tunnel run
```

See [OpenAI + ChatGPT setup](openai-chatgpt.md) for Platform/ChatGPT configuration.

## Upstream MCP servers

```bash
cmcp mcp --help
cmcp mcp server --help
cmcp mcp server list
cmcp mcp server show <id>
cmcp mcp server status <id>
cmcp mcp server tools <id>
cmcp mcp server auth --help
```

Use the server subcommands to add, inspect, update, or remove upstream MCP definitions supported by the current binary.

See [MCP and upstreams](mcp.md).

## Status

```bash
cmcp status
```

Status is the main read-only overview for:

- config root
- runtime state
- foreground/managed service state
- service scope/backend/ID
- PID/start information
- registered workspaces
- upstream servers

## Isolated instances

One command:

```bash
cmcp --config-dir /tmp/cmcp-test status
```

Environment:

```bash
CHATGPT_MCP_CONFIG_DIR=/tmp/cmcp-test cmcp status
```

Always use an isolated config root when running destructive test/dev flows such as `init`, `uninit`, `config set`, workspace registration, or tunnel configuration.

## CLI logging modes

Default:

```bash
cmcp status
```

Operational context:

```bash
cmcp status --verbose
```

Full diagnostics:

```bash
cmcp status --debug
```

Machine-readable:

```bash
cmcp status --log-format=json
```

Visibility flags also apply when replaying persistent runtime logs.
