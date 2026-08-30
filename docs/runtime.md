# Runtime and services

`chatgpt-mcp` can run either attached to the current terminal with `serve`, or as an OS-managed background runtime with `up`.

## Foreground: `serve`

```bash
cmcp serve
```

Use this for:

- interactive development
- quick testing
- seeing current terminal output directly
- one-off `--expose` overrides

The process remains attached to the current terminal/session. Closing an SSH session can terminate a foreground runtime.

Examples:

```bash
cmcp serve
cmcp serve --verbose
cmcp serve --debug
cmcp serve --log-format=json
cmcp serve --expose=eth0
```

## Managed runtime: `up`

```bash
cmcp up
```

`up` resolves the selected config root, creates or updates the appropriate OS service definition, starts it, waits for the local runtime control channel, and prints enough context for the user to understand what was installed.

The service definition stores an absolute `--config-dir`, so it does not depend on the environment of a future login session.

`up` is idempotent:

- missing service → install + start
- installed but stopped → start
- installed with stale definition → update + restart as needed
- already correct and running → report the existing runtime

If a foreground `serve` is already using the selected config root, `up` refuses to adopt or kill it.

## Stop/remove: `down`

```bash
cmcp down
```

`down` stops and removes the managed service for the selected scope. It does **not** delete:

- configuration
- workspaces
- checkpoints
- OAuth/upstream state
- runtime logs

Use `cmcp uninit` only when you intentionally want to remove the selected local config/state root.

## Linux service behavior

### Normal user

```bash
cmcp up
```

Uses:

```text
systemd --user
```

The MCP process runs as the current user.

If user lingering is disabled, `chatgpt-mcp` warns that the user service manager may stop after the final login/SSH session ends. It does not enable lingering automatically.

### With sudo

```bash
sudo cmcp up
```

Uses a system-level systemd unit and starts with the machine.

The service itself still runs the MCP process as the invoking user from `SUDO_USER`; `chatgpt-mcp` does not run the MCP runtime as root.

The default config root is also resolved for the invoking user instead of `/root` unless an explicit `--config-dir` or environment override is supplied.

Stop/remove the matching system service with:

```bash
sudo cmcp down
```

Normal `cmcp down` does not silently remove the system-scope service.

## macOS service behavior

Normal:

```bash
cmcp up
```

uses a user LaunchAgent.

With sudo:

```bash
sudo cmcp up
```

uses a system LaunchDaemon, but the daemon's `UserName` remains the invoking user.

Use the same privilege/scope to remove it with `down`.

## Windows service behavior

```powershell
cmcp up
```

uses a per-user Task Scheduler task.

The task uses the current user, an interactive token, and least privilege. It does not run as LocalSystem and does not store the user's password.

An elevated terminal does not switch `cmcp up` to a LocalSystem/system-service mode.

## Service identity and parallel config roots

Managed service identity includes a stable hash of the canonical config root. This lets different config roots coexist without colliding.

For example:

```bash
cmcp up
cmcp --config-dir ~/cmcp-dev up
cmcp --config-dir ~/cmcp-test up
```

Each selected root maps to a distinct managed service.

On Linux/macOS, user and system scopes also have distinct service identities.

## Inspect status

```bash
cmcp status
```

Status reports information such as:

- selected config root
- initialized/uninitialized state
- runtime running/stopped
- foreground vs managed runtime
- service backend
- user/system scope where applicable
- service ID
- PID and start information
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
cmcp config set server.port 41021
```

Then apply it to the running process:

```bash
cmcp config reload
```

Reload does not restart the process.

Live changes include:

- authentication state
- features
- filesystem permission roots
- tunnel configuration

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

### Replay

```bash
cmcp logs
cmcp logs -n 200
cmcp logs --verbose
cmcp logs --debug
cmcp logs --log-format=json
```

### Follow

```bash
cmcp logs -f
cmcp logs follow
cmcp logs --debug -f
```

Follow first reads matching history and then switches to the authenticated live runtime event stream. It does not poll the journal file.

### Filters

```bash
cmcp logs --since 30m
cmcp logs --until 2026-08-31T12:00:00+07:00
cmcp logs --level warn
cmcp logs --component SERVER,TUNNEL
cmcp logs --workspace ws_...
cmcp logs --workspace ~/projects/my-project
cmcp logs --tool run_command --status error
cmcp logs --source tunnel
cmcp logs --event 'tool.call.*'
cmcp logs --grep timeout
```

Filters operate on structured event fields before CLI rendering.

### Journal location

```bash
cmcp logs path
```

### Clear logs

```bash
cmcp logs clear --force
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

Use `--verbose` for operational context and `--debug` for full diagnostics including timestamps, levels, components, event names, IDs, TLS/proxy metadata, and low-level tunnel/runtime events.

Use `--log-format=json` when logs will be consumed by automation.
