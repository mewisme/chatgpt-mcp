# Runtime and services

`chatgpt-mcp` can run either attached to the current terminal with `serve`, or as an OS-managed background runtime with `up`.

## Foreground: `serve`

```bash
cgm serve
```

Use this for:

- interactive development
- quick testing
- seeing current terminal output directly
- one-off `--expose` overrides

The process remains attached to the current terminal/session. Closing an SSH session can terminate a foreground runtime.

Examples:

```bash
cgm serve
cgm serve --verbose
cgm serve --debug
cgm serve --log-format=json
cgm serve --expose=eth0
```

## Managed runtime: `up`

```bash
cgm up
```

`up` resolves the selected config root, creates or updates the appropriate OS service definition, starts it, waits for the local runtime control channel, and prints enough context for the user to understand what was installed.

The startup summary includes the runtime session ID, PID, MCP/admin endpoints, and tunnel state. Tunnel state distinguishes whether it is enabled, whether ID/API-key setup is complete, and whether the live tunnel is connected, connecting, reconnecting, or stopped. The tunnel ID is shown when configured; the API key is never printed.

The service definition stores an absolute `--config-dir`, so it does not depend on the environment of a future login session.

`up` is idempotent:

- missing service → install + start
- installed but stopped → start
- installed with stale definition → update + restart as needed
- already correct and running → report the existing runtime

If a foreground `serve` is already using the selected config root, `up` refuses to adopt or kill it.

## Stop/remove: `down`

```bash
cgm down
```

`down` stops and removes the managed service for the selected scope. It does **not** delete:

- configuration
- workspaces
- checkpoints
- OAuth/upstream state
- runtime logs

Use `cgm uninit` only when you intentionally want to remove the selected local config/state root.

## Linux service behavior

### Normal user

```bash
cgm up
```

Uses:

```text
systemd --user
```

The MCP process runs as the current user.

If user lingering is disabled, `chatgpt-mcp` warns that the user service manager may stop after the final login/SSH session ends. It does not enable lingering automatically.

### Machine-level scope

```bash
cgm up --system
```

Uses a system-level systemd unit and starts with the machine. From a normal user shell, the CLI detects user scope and automatically re-executes its stable absolute launcher through `sudo`, avoiding `sudo secure_path` issues when `cgm` lives under `~/.local/bin`.

The service itself still runs the MCP process as the invoking user from `SUDO_USER`; `chatgpt-mcp` does not run the MCP runtime as root.

The default config root is also resolved for the invoking user instead of `/root` unless an explicit `--config-dir` or environment override is supplied.

Stop/remove the matching system service with:

```bash
cgm down --system
```

Normal `cgm down` does not silently remove the system-scope service. Direct `sudo /absolute/path/cgm up|down` remains supported for compatibility.

## macOS service behavior

Normal:

```bash
cgm up
```

uses a user LaunchAgent.

Machine-level:

```bash
cgm up --system
```

uses a system LaunchDaemon, but the daemon's `UserName` remains the invoking user. The CLI elevates through `sudo` automatically when needed.

Use the same privilege/scope to remove it with `down`.

## Windows service behavior

```powershell
cgm up
```

uses a per-user Task Scheduler task.

The task uses the current user, an interactive token, and least privilege. It does not run as LocalSystem and does not store the user's password.

An elevated terminal does not switch `cgm up` to a LocalSystem/system-service mode.

## Service identity and parallel config roots

Managed service identity includes a stable hash of the canonical config root. This lets different config roots coexist without colliding.

For example:

```bash
cgm up
cgm --config-dir ~/cgm-dev up
cgm --config-dir ~/cgm-test up
```

Each selected root maps to a distinct managed service.

On Linux/macOS, user and system scopes also have distinct service identities.

## Inspect status

```bash
cgm status
```

Status reports information such as:

- selected config root
- initialized/uninitialized state
- runtime running/stopped
- foreground vs managed runtime
- service backend
- user/system scope where applicable
- service ID
- current runtime session ID
- PID and start information
- tunnel enabled/configured/live state and tunnel ID
- workspaces and upstream state

A normal user can inspect a system-managed runtime. Mutations such as removing a system service still require the matching privilege.

## Runtime control channel

A running server creates a loopback-only authenticated runtime control endpoint associated with the selected config root.

It supports internal operations used by CLI commands such as:

- live config reload
- runtime status
- graceful shutdown
- live event streaming
- safe log clearing

The control token is stored under the protected config/state root and is not intended as a user-facing API credential.

## Live config reload

Persist a change:

```bash
cgm config set server.port 41021
```

Then apply it to the running process:

```bash
cgm config reload
```

Reload does not restart the process.

Live changes include:

- authentication state
- features
- filesystem permission roots
- cluster federation
- tunnel configuration

When cluster federation is enabled, the runtime also supervises its relay connection. Relay loss does not stop the MCP HTTP runtime: pending cluster RPCs fail closed, the node reconnects with bounded backoff, and current workspace/catalog state is re-advertised after recovery. See [Cluster federation](cluster.md).

Network-affecting changes such as MCP/admin port or exposure cause listener rebind inside the same process.

Listener reload is transactional: if the new listeners cannot be opened, the previous listener set is restored.

A foreground `serve --expose=...` command-line override remains authoritative across config reloads.

## Structured runtime logs

Runtime events are persisted under the selected config root:

```text
<config-root>/logs/runtime.jsonl
```

Default rotation:

```text
10 MiB per file
5 files retained
```

Events are persisted before terminal visibility filtering, so a service started normally can later be inspected at verbose or debug detail.

Every runtime process has a stable `run_id` for its lifetime. New runtimes also write `runtime.session.started` and `runtime.session.ended` journal markers. Text replay uses the same ID to print a clear session separator even when the requested tail/filter begins in the middle of a session.

### Replay

```bash
cgm logs
cgm logs -n 200
cgm logs --verbose
cgm logs --debug
cgm logs --log-format=json
```

Text replay shows `HH:MM:SS` on primary event lines by default. Indented detail lines do not repeat the timestamp. Normal command output such as `cgm status`, `cgm up`, or foreground `cgm serve` remains timestamp-free, including debug mode.

Hide replay timestamps when desired:

```bash
cgm logs --no-time
```

The session separator includes the local date and time; individual event lines only repeat `HH:MM:SS`.

Filter a single runtime session using the full ID or the shortened prefix printed in the session separator / `cgm status` / `cgm up`:

```bash
cgm logs --session run_a1b2c3d4e5f6
cgm logs --session run_a1b2c3d4e5f6 -f
```

JSON replay does not emit the text separator; each JSON event instead carries `run_id`, `pid`, and managed service metadata where applicable.

### Follow

```bash
cgm logs -f
cgm logs follow
cgm logs --debug -f
```

Follow first reads matching history and then switches to the authenticated live runtime event stream. It does not poll the journal file.

### Filters

```bash
cgm logs --since 30m
cgm logs --until 2026-08-31T12:00:00+07:00
cgm logs --session run_a1b2c3d4e5f6
cgm logs --level warn
cgm logs --component SERVER,TUNNEL
cgm logs --workspace ws_...
cgm logs --workspace ~/projects/my-project
cgm logs --tool run_command --status error
cgm logs --source tunnel
cgm logs --event 'tool.call.*'
cgm logs --grep timeout
```

Filters operate on structured event fields before CLI rendering.

### Journal location

```bash
cgm logs path
```

### Clear logs

```bash
cgm logs clear --force
```

If the runtime is running, clearing is performed through the runtime control channel so the active writer can safely reset journal state. If stopped, files are cleared directly.

Log reading/following is considered read-only in MCP tool execution context; clearing is a control-plane mutation and is denied there.

## CLI-first rendering

Normal text output intentionally avoids noisy level/component prefixes. Stable markers are used instead:

```text
✓ success / ready
! warning
× error
· information
→ action
```

Use `--verbose` for operational context and `--debug` for full diagnostics including levels, components, event names, IDs, TLS/proxy metadata, and low-level tunnel/runtime events. Historical/live `cgm logs` adds timestamps independently of visibility mode.

Use `--log-format=json` when logs will be consumed by automation.
