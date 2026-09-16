# Runtime and operations

Use `serve` for a foreground process and `up` for the normal managed background runtime.

| Mode | Command | Best for |
| --- | --- | --- |
| Foreground | `cgm serve` | development, one-off testing, direct terminal output |
| Managed | `cgm up` | normal daily use, remote servers, restartable background operation |

## Foreground runtime

```bash
cgm serve
```

The process stays attached to the current terminal. Closing that terminal or SSH session can stop it.

Useful variants:

```bash
cgm serve --verbose
cgm serve --debug
```

The default ChatGPT setup still uses OpenAI Secure MCP Tunnel; a foreground runtime starts every enabled attached tunnel instance against the same local runtime.

## Managed runtime

Start or reconcile the managed service:

```bash
cgm up
```

Inspect it:

```bash
cgm status
```

Restart it:

```bash
cgm restart
```

Stop and remove the managed service definition:

```bash
cgm down
```

`down` preserves configuration, workspaces, secrets, and runtime logs. Use `cgm uninit` only when you intentionally want to remove the selected local config/state root.

`up` is idempotent: it creates a missing service, starts a stopped service, reconciles a stale definition, or reports an already healthy runtime.

If a foreground `serve` process already owns the selected config root, `up` refuses to silently take it over.

## Service scope by platform

### Linux

```bash
cgm up
```

uses a user systemd service.

For a machine-level service that starts with the machine:

```bash
cgm up --system
```

The CLI may elevate the service-management operation through `sudo`, but the `chatgpt-mcp` runtime itself is configured to run as the invoking user rather than root.

On remote Linux, use `--system` when a user service would otherwise stop after the final login because user lingering is disabled.

### macOS

`cgm up` uses a user LaunchAgent. `cgm up --system` uses a system LaunchDaemon while keeping the runtime under the invoking user's identity.

### Windows

`cgm up` uses a per-user Task Scheduler task with least privilege. It does not run the runtime as LocalSystem.

## Status

```bash
cgm status
```

Use status as the first operational overview. It reports the selected config root, runtime/service state, transport state, aggregate tunnel counts plus per-instance state, relevant endpoints, and registered resource summaries.

For tunnel-specific state:

```bash
cgm tunnel list
cgm tunnel status [tunnel_id]
```

## Logs

Runtime events are persisted under the selected config root and can be replayed or followed live.

History:

```bash
cgm logs
cgm logs -n 200
cgm logs --verbose
cgm logs --debug
```

Follow:

```bash
cgm logs -f
```

Useful filters:

```bash
cgm logs --since 30m
cgm logs --level warn
cgm logs --component SERVER,TUNNEL
cgm logs --workspace ws_...
cgm logs --tool run_command --status error
cgm logs --grep timeout
```

Locate or clear the journal:

```bash
cgm logs path
cgm logs clear --force
```

Use `--log-format=json` when consuming event output programmatically. See [CLI reference](cli-reference.md#logs) for the full filter surface.

## Configuration changes

Supported configuration mutations are applied to the running runtime through its local control plane. Network changes such as port or exposure updates are rebound transactionally; if a new listener cannot be opened, the previous working listener set is retained and the local mutation reports failure.

If the runtime is stopped, persisted changes take effect on the next start.

```bash
cgm config set server.port 41021
cgm config verify
```

See [Configuration](configuration.md).

## Tunnel lifecycle

The normal managed runtime automatically starts all enabled attached OpenAI Secure MCP Tunnel instances. They share one `tools.Runtime` but own independent tunnel sessions, reconnect loops, metadata, errors, and lifecycle state.

Useful commands:

```bash
cgm tunnel list
cgm tunnel status [tunnel_id]
cgm tunnel enable tunnel_...
cgm tunnel disable tunnel_...
cgm tunnel start tunnel_...
cgm tunnel stop tunnel_...
cgm tunnel run tunnel_...
```

`tunnel run <id>` is a foreground tunnel-only operation; normal `serve` / `up` own the usual integrated lifecycle. Runtime reload reconciles the collection differentially: adding/removing/changing one tunnel does not restart unrelated tunnel clients.

Readiness is transport-wide: direct MCP HTTP can make the runtime usable on its own, otherwise at least one enabled/configured tunnel must become ready. Later failure of one tunnel is reported as degraded while the process keeps other tunnels running/reconnecting.

### Identity and labels

`Source=tunnel` means the request arrived over Secure MCP Tunnel. It is not which tunnel. Correlation, filters, approvals, and executions use `TunnelID` (`tunnel_...`). `TunnelName` is optional cached display metadata and is never fetched on the tool-call hot path.

Dense CLI/TUI/Admin lists prefer the cached name. Duplicate names get a short-ID suffix such as `Production · c3330bcd`. The full ID stays in detail, copy/debug, search, and unnamed fallbacks.

`--source tunnel` matches every tunnel ingress. `--tunnel <id-or-label>` selects one instance.

See [OpenAI + ChatGPT](openai-chatgpt.md) for setup.

## Updates

Check without changing the installation:

```bash
cgm upgrade check
```

Upgrade a managed direct installation:

```bash
cgm upgrade
```

Install an exact version, including an intentional downgrade:

```bash
cgm upgrade --version vX.Y.Z
```

Keep a running managed runtime on its current in-memory version until a later restart:

```bash
cgm upgrade --no-restart
```

Direct managed updates stage the target version, verify release checksums, switch the stable installation target, restart a matching managed runtime when requested, and roll back if the new runtime cannot become ready. A foreground `serve` process is never killed by the updater; restart it manually to load the new binary.

Homebrew and Scoop installations remain owned by their package managers. Development/`go install` binaries do not silently adopt the managed direct-update flow.

## Multiple config roots

Each selected config root is an independent runtime instance:

```bash
cgm up
cgm --config-dir ~/cgm-dev up
cgm --config-dir ~/cgm-test up
```

Configuration, workspaces, secrets, logs, runtime control state, and service identity remain scoped to the selected root.

## Interactive operation

Open the full-screen Command Center:

```bash
cgm tui
```

The TUI can inspect runtime state, logs, requests, workspaces, tunnel state, configuration, and lifecycle actions without replacing the scriptable CLI. See [TUI Command Center](tui.md).

## Troubleshooting

Start with:

```bash
cgm status
cgm tunnel status
cgm logs --debug -n 200
```

Then use [Troubleshooting](troubleshooting.md) for symptom-specific fixes.
