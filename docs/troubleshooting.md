# Troubleshooting

Use this guide for common runtime, service, tunnel, configuration, and connectivity failures.

Start with these three commands:

```bash
cgm status
cgm tunnel status
cgm logs --debug -n 200
```

## MCP session needs to work across multiple workspaces

No workspace-switch command is required. A single MCP session may call workspace-scoped tools against multiple registered projects as long as every call supplies the intended valid `workspace_id`.

If a call targets the wrong project, correct the `workspace_id` on that call rather than changing global session state. The runtime keeps filesystem scope, project context, memory, shell cwd, REPL state, checkpoints, and approvals isolated per workspace. The Activity page shows the target workspace, a short session fingerprint, whether that workspace was a `new` or `existing` access for the session, and the session workspace count; the raw MCP session ID is never exposed.

## Workspace ID changed after a registry v2 upgrade

Current workspace IDs are stable hashes of canonical workspace paths. Registry v2 instance-scoped IDs are migrated to the stable path-based ID and retained as aliases, so existing conversations using the old ID continue to resolve to the same workspace. If an ID is genuinely unknown, re-run `cgm workspace list` or register the canonical path again; the runtime never falls back to another workspace.

## ChatGPT cannot see the tunnel

Check:

1. The tunnel exists in OpenAI Platform Tunnels.
2. The tunnel is associated with the target ChatGPT workspace, not only a Platform organization.
3. Your Platform principal has **Tunnels Read + Use**.
4. Your ChatGPT user has Developer Mode access enabled.
5. `cgm` is running.
6. `cgm tunnel status` shows the intended `tunnel_...` ID.
7. Tunnel logs do not show authentication or polling failures.

Useful:

```bash
cgm logs --component TUNNEL --debug -n 200
```

OpenAI documents that role/permission changes can take time to propagate, so retry after the new assignment has become active.

See [OpenAI + ChatGPT setup](openai-chatgpt.md).

## Tunnel authentication fails / 403

The runtime API key principal likely does not have the required tunnel permissions or is scoped to the wrong organization/tunnel.

For normal runtime use, grant:

```text
Tunnels Read + Use
```

Tunnel creation/editing requires:

```text
Tunnels Read + Manage
```

Do not replace the runtime key with a Platform Admin API key.

Reconfigure when necessary:

```bash
cgm tunnel configure \
  --enabled \
  --id tunnel_... \
  --api-key 'sk-...'
```

Then:

```bash
cgm tunnel status
cgm logs --component TUNNEL --debug -f
```

## ChatGPT Scan Tools fails

Verify the local runtime is alive while scanning:

```bash
cgm status
cgm tunnel status
```

Keep it running for both tool discovery and normal calls.

If using foreground mode, do not close the terminal running:

```bash
cgm serve
```

For background operation use:

```bash
cgm up
```

Then inspect:

```bash
cgm logs --debug -f
```

## `cgm serve` dies after SSH disconnect

`serve` is a foreground process and is attached to the SSH/session environment.

Use a managed service instead:

```bash
cgm up
```

On Linux this is a user systemd service. If the CLI warns that lingering is disabled, the user manager may stop after the final login session ends.

For a machine-level service that starts at boot:

```bash
cgm up --system
```

The CLI automatically elevates through `sudo` when needed, using its absolute launcher path so `sudo secure_path` does not need to contain `~/.local/bin`. The MCP process still runs as the invoking user, not root.

## `cgm up` says a foreground runtime is already running

`up` intentionally refuses to adopt or kill a foreground `serve` process.

Stop the foreground process normally, then run:

```bash
cgm up
```

This prevents service management from unexpectedly taking ownership of a manually started runtime.

## `cgm down` cannot remove a system service

On Linux/macOS, system scope and user scope are distinct.

If the service was created with:

```bash
cgm up --system
```

remove it with:

```bash
cgm down --system
```

Normal `cgm down` only manages the normal user-scope service.

## Linux user service warns about lingering

A user-level systemd service can depend on the user manager lifecycle.

`chatgpt-mcp` reports the condition but does not change OS lingering policy automatically.

Options:

- keep the user service and manage lingering yourself according to server policy
- use `cgm up --system` for a machine-level systemd unit that starts at boot

## Config changes are not visible in the running server

`config set` persists the change; it does not implicitly replace the running process state in every case.

Apply persisted configuration:

```bash
cgm config reload
```

Verify:

```bash
cgm status
cgm config get
```

## `config reload` fails after changing a port

The new listener may be unavailable.

Check:

```bash
cgm logs --component SERVER --debug -n 200
```

Listener reload is transactional. A failed new bind should restore the previous working listener set rather than leaving the process unavailable.

Choose a free port, persist it, and reload again:

```bash
cgm config set server.port 41021
cgm config reload
```

## Config format mismatch or manual edit failure

Run:

```bash
cgm config verify
```

If you intentionally want one format across the managed state tree:

```bash
cgm config convert json
cgm config convert yaml
cgm config convert toml
```

Conversion preflights structured state before mutation.

## I accidentally used the real config root in a test

Stop the test process immediately and inspect:

```bash
cgm status
cgm config path
```

For future test/dev runs, always use:

```bash
cgm --config-dir /tmp/cgm-test ...
```

or:

```bash
CHATGPT_MCP_CONFIG_DIR=/tmp/cgm-test cgm ...
```

Repository tests and release smoke are designed to use explicit non-default roots.

## Workspace path is denied

Check the registered workspace and additional roots:

```bash
cgm workspace list
cgm workspace show ws_...
cgm workspace access list ws_...
cgm config get permissions.allow_dirs
```

Register the correct root if needed:

```bash
cgm workspace register /absolute/path/to/project
```

Or grant a narrow extra root:

```bash
cgm workspace access add ws_... /absolute/path/to/cache
```

Symlink escapes remain denied even when a textual path appears to be inside an allowed directory.

## MCP tool cannot run `cgm config set`, `up`, or other mutations

This is intentional.

MCP tool execution context allows read-only inspection but denies control-plane self-modification such as:

- `up` / `down`
- config mutation/reload
- workspace registration/access grants
- auth changes
- tunnel configuration
- upstream changes
- `logs clear`
- `init` / `uninit`

Make control-plane changes from a trusted user terminal instead.

## Direct MCP request gets 401/403

Inspect authentication:

```bash
cgm auth status
```

Create/rotate an MCP token if needed:

```bash
cgm auth mcp create
```

Then send:

```http
Authorization: Bearer <mcp-token>
```

Do not confuse this MCP token with the OpenAI tunnel runtime API key.

## Wildcard exposure is rejected

`0.0.0.0` exposure requires both MCP and Admin authentication with configured tokens.

Inspect:

```bash
cgm auth status
```

Create credentials if appropriate:

```bash
cgm auth mcp create
cgm auth admin create
```

Then configure wildcard exposure only if you genuinely need direct network ingress.

For ChatGPT connectivity, prefer Secure MCP Tunnel and keep the local listener private.

## Logs are too quiet

Replay verbose events:

```bash
cgm logs --verbose -n 200
```

Full diagnostics:

```bash
cgm logs --debug -n 200
```

Live diagnostics:

```bash
cgm logs --debug -f
```

The runtime persists debug-visible structured events even when the service was originally started without `--debug`.

## Logs are too large

The runtime journal rotates automatically.

Locate it:

```bash
cgm logs path
```

Clear current and rotated logs intentionally:

```bash
cgm logs clear --force
```

When the runtime is alive, clearing is coordinated through the runtime control channel.

## Windows upgrade/install problem while the runtime is active

Current Windows installer layout uses immutable version directories and a stable `current` directory junction so normal upgrades do not overwrite the executable currently held open by the managed runtime.

If upgrading from a legacy installation whose `current` path is still a real directory and Windows cannot migrate it because files are locked:

```powershell
cgm down
```

then rerun the installer. Start the managed runtime again afterward:

```powershell
cgm up
```

## Upstream MCP refresh fails

Upstream proxy replacement is atomic. A failed discovery/schema refresh should leave the previous proxy catalog active.

Inspect:

```bash
cgm status
cgm mcp --help
cgm logs --debug -n 200
```

Fix the upstream endpoint/auth/discovery issue, then retry the relevant upstream configuration action.

## Still stuck

Collect these without copying secrets:

```bash
cgm --version
cgm status
cgm tunnel status
cgm auth status
cgm config verify
cgm logs --debug -n 200
```

Before sharing logs, review them for project paths, command output, or other environment-specific information you do not want to disclose.
