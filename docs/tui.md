# TUI Command Center

`cgm tui` is the human-operated terminal interface for `chatgpt-mcp`. The normal CLI remains the stable surface for scripts and automation.

```text
cgm ...   scriptable CLI
cgm tui   interactive Command Center
```

## Start

```bash
cgm tui
```

`cgm tui` is a small core launcher. The Command Center itself is the `tui` core plugin (`terminal-ui/default`). If that plugin is missing from an **installed** `cgm`, the command fails with `cgm plugin install tui`; if it is installed but disabled, enable it with `cgm plugin enable tui`. From a source checkout, `go run . tui` uses the repository-local `local-dev` plugin automatically ([Development](development.md)).

The TUI requires a real terminal. Redirected/non-TTY invocation fails instead of writing alternate-screen output into a pipe.

Deep-link directly to a page or resource when useful:

```bash
cgm tui workspace
cgm tui workspace ws_...
cgm tui mcp github
cgm tui tunnel
cgm tui requests
cgm tui logs
cgm tui config
cgm tui runtime
cgm tui guide
```

## Global navigation

| Key | Action |
| --- | --- |
| `Ctrl+K` | open Commands for actions, pages, resources, and Guide topics |
| `Alt+Left` / `Alt+Right` | cycle top-level pages |
| `Esc` | close the nearest overlay/child page, then navigate back/exit |
| `Backspace` | navigate back when an input is not consuming the key |

Page-local shortcuts are shown directly above the application footer. Press `?` where available to expand additional bindings.

Mouse clicks and scrolling follow the same actions and validation paths as keyboard input.

## Commands

Press `Ctrl+K` and search for an action, resource, page, ID, or canonical CLI path.

Examples:

```text
workspace register
upstream server add
request approve
config verify
restart
guide logs
```

Commands is the primary discovery surface. There is no separate Quick Open workflow.

## Main areas

The top-level navigation covers:

```text
Workspaces | MCP | Tunnel | Requests | Logs | Config | Instruction | Runtime
```

Child resources remain owned by their parent area. Additional resources such as workspace containers, managed tunnels, About/build information, and embedded Guide topics are reachable through Commands or deep links.

## Editors and confirmations

Create/edit/configure workflows use full-page editors so long forms remain usable in small terminals.

Common behavior:

- `Enter` advances structured fields/sections and performs the editor action on the final visible field.
- Multiline inputs keep `Enter` for newlines and use `Ctrl+Enter` for save/create/apply actions.
- Path fields use `Ctrl+O` to switch between picker and manual input where supported.
- Sensitive fields use password-style input and do not expose persisted secrets.
- Failed operations keep the current draft.
- Leaving an unsaved editor requires explicit discard confirmation.
- Destructive actions require confirmation.

## Workspaces

The Workspaces area manages concrete `ws_*` project roots, additional access directories, workspace containers, and Project Context previews.

A workspace detail can relocate a project after its directory has already moved. Relocation updates the trusted registered root and keeps the workspace ID; it does not move project files itself.

Workspace containers (`wsc_*`) are grouping/orchestration resources, not filesystem scopes. See [Workspaces](workspaces.md).

## MCP

The MCP area manages upstream MCP servers, health/tool discovery, and tool exposure.

Server creation supports both form-driven setup and canonical JSON input. Sensitive environment/header values remain managed as secrets rather than being echoed into normal detail views.

See [MCP and upstreams](mcp.md).

## Tunnel

Tunnel is collection-first. Its top-level browser lists attached local tunnel instances and each detail is scoped by tunnel ID. Enable/disable/start/stop/detach act on only that instance. `n` attaches a local runtime key (`cgm tunnel add`); `e` edits that instance (`cgm tunnel update`) and a blank runtime key keeps the current secret.

Rows prefer the cached tunnel name. Duplicate names get a short-ID suffix. The full ID stays in detail, search, and unnamed fallbacks.

Managed Tunnels and admin profiles are separate resources. `cgm tunnel attach` from Managed Tunnels adds a local instance instead of replacing an existing attachment. The Admin UI Tunnel page exposes the same local attach/edit/lifecycle and admin-profile update flows. The normal ChatGPT runtime credential is a restricted **Tunnels Read + Use** key per instance; admin-profile keys are management-only. See [OpenAI + ChatGPT](openai-chatgpt.md).

## Requests

Requests is the approval inbox for guarded actions. Pending requests can also appear in the global live approval dialog so a local operator can review the exact action without leaving the current page. The dialog can approve once or grant similar commands for all MCP sessions for one hour when the request includes a similar-command pattern. The Admin UI has the same approve/deny/reason/similar-grant controls on a global Requests page and each workspace Requests tab. When no TUI reviewer is open, the runtime may also send a best-effort desktop notification; see [Desktop notifications](configuration.md#desktop-notifications).

Approval does not create a general shell bypass; it authorizes the runtime-defined action/retry scope. See [Security](security.md#control-guard-approvals-and-self-grant-prevention).

## Logs

Logs has three tabs:

```text
Runtime | Command Execution | Tool Calls
```

All three support two views:

- **Browser** — inspect individual records and open details.
- **Timeline** — follow the chronological stream.

Use:

| Key | Action |
| --- | --- |
| `v` | switch Browser / Timeline |
| `m` | choose Stream Mode |
| `Space` | pause/resume follow |
| `r` | refresh/reconnect where applicable |

Stream Mode controls the visible scope for Runtime, Command Execution, and Tool Calls. Command Execution can additionally select Process view for a workspace; the other tabs do not expose Process mode.

Tunnel-originated rows show the cached tunnel label, not `Source=tunnel` as identity. Search matches both the label and the canonical tunnel ID. The Logs filter editor has a dedicated Tunnel field plus the transport Source field.

### Runtime

Runtime combines persistent journal history with the live runtime event stream. Tool-call lifecycle events are presented in the dedicated Tool Calls tab rather than duplicated into Runtime history.

### Command Execution

Command Execution follows the runtime execution feed and keeps stdout/stderr in event order. Browser view makes individual executions easy to inspect; Timeline is better for watching interleaved activity live.

### Tool Calls

Tool Calls shows authoritative tool-call lifecycle records. Browser detail renders the complete structured call body; Timeline renders request/result/error blocks in chronological order.

### Follow behavior

While a Logs page is active, follow keeps the selected view at the newest visible item/event. Navigating a Browser row away from the tail pauses follow so the selection does not get pulled out from under the user. Press `Space` to return to follow.

Leaving Logs for another top-level page closes its live feeds. Returning reconstructs the page from fresh history/snapshots and **resumes follow automatically** instead of restoring a stale paused stream.

## Config and Instruction

Config is schema-driven. Use it for typed configuration editing and storage/maintenance operations; exhaustive configuration semantics remain available through `cgm config explain` and [Configuration](configuration.md).

Instruction manages Global Context, managed rules, and detected instruction sources used by Project Context assembly.

## Runtime

Runtime is the operational control surface for service state, authentication, install/update actions, alias state, and version/build information. Direct MCP HTTP token reveal/copy/rotate live here and reuse the stored token; admin rotation remains one-time plaintext.

For scripts or remote automation, use the equivalent CLI commands instead. See [Runtime and operations](runtime.md).

## Embedded Guide

The Markdown tree under [`tuiguide/`](tuiguide/) is embedded into the binary as contextual help.

Open Commands (`Ctrl+K`) and search for **Guide**, or deep-link directly:

```bash
cgm tui guide
cgm tui guide logs
cgm tui guide mcp
```

The embedded Guide intentionally explains the current page/editor instead of duplicating the complete public documentation.

## Scripting and automation

Do not automate the full-screen TUI. Use normal CLI commands and structured output instead:

```bash
cgm workspace list --json
cgm upstream server list --json
cgm config verify
cgm status
```

Use `cgm <command> --help` and the [CLI reference](cli-reference.md) for the scriptable interface.
