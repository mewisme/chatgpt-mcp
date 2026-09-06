# TUI Command Center

`chatgpt-mcp` has two user interfaces with different jobs:

```text
cgm ...       stable scriptable CLI
cgm tui       full-screen interactive terminal application
```

Use normal CLI commands in scripts, automation, CI, and any workflow that needs deterministic text or structured output. Use `cgm tui` when a human wants to browse resources, run actions, edit configuration, review requests, or inspect runtime state interactively.

## Start the Command Center

```bash
cgm tui
```

The TUI requires terminal stdin and stdout. Redirected/non-TTY invocation fails clearly instead of emitting alternate-screen or ANSI output into a pipe.

```bash
cgm tui --help
```

The application uses the terminal's current size, adapts to light/dark backgrounds with Charm-native styles, and supports both keyboard and mouse interaction. Every mouse operation has a keyboard equivalent and follows the same action, validation, and confirmation path.

## Global shortcuts

| Key | Action |
| --- | --- |
| `Ctrl+P` | open the Command Palette |
| `Ctrl+O` | open Quick Open for pages and resources |
| `Alt+Left` / `Alt+Right` | cycle top-level pages with wrap-around |
| `Esc` | close the top overlay, navigate back, or open exit confirmation at the root |
| `Backspace` | navigate back when route history is available |

Lists, forms, tabs, and detail views expose their own contextual key hints. Browser lists keep up to five custom actions in the compact hint row; when a page has more than five custom actions, those actions are hidden from the compact row and are available through `? more`. Expanded help stays expanded across automatic refreshes and list rebuilds. The app footer stays focused on global navigation instead of duplicating local controls.

## Command Palette

Press `Ctrl+P` to search the shared action registry. Search matches action titles, categories, keywords, resource context, and canonical CLI command paths, so CLI knowledge transfers directly to the TUI.

Examples:

```text
workspace register
mcp server tools
config verify
auth mcp create
restart
```

Use `Up` / `Down` to move, `Enter` to run the selected action, and `Esc` to close. Results can also be selected with the mouse and the wheel moves through the result list.

Actions execute typed application/domain operations directly. The TUI never shells out to `cgm ...` to implement an action.

## Quick Open

Press `Ctrl+O` to navigate instead of execute. Quick Open indexes pages and available resources such as registered workspaces, workspace containers, MCP servers, and managed tunnels.

Typical searches:

```text
github       -> MCP server github
ws_abcd...   -> registered workspace
logs         -> Logs page
config       -> Config page
```

Use `Up` / `Down`, `Enter`, `Esc`, mouse click, and mouse wheel the same way as the Command Palette.

## Deep links

`cgm tui [path...]` opens a page or resource directly. Useful entry points include:

```bash
cgm tui
cgm tui workspace
cgm tui workspace ws_...
cgm tui containers wsc_...
cgm tui mcp github
cgm tui tunnel
cgm tui tunnels tunnel_...
cgm tui requests req_...
cgm tui logs
cgm tui config
cgm tui runtime
cgm tui about
```

Aliases accepted by the route parser include `ws`, `cfg`, `req`, `status`, and `version`. Unknown paths fail instead of silently opening an unrelated page.

Resource routes keep their owning top-level page active. For example, a workspace container still belongs to the Workspaces navigation section, while a managed tunnel belongs to Tunnel.

## Forms and confirmations

Create/configure/edit workflows use Huh forms with the Charm theme and the same validators used by the underlying domain/config operations where possible.

- Current values are prefilled for edit flows.
- Sensitive values use password-style fields and remain redacted after persistence.
- Validation errors stay on the relevant field instead of submitting partial state.
- Cancelling a form does not persist mutations.
- Destructive actions such as unregister, remove, delete, clear, logout, token rotation, and similar lifecycle changes require explicit confirmation.
- Long-running operations execute asynchronously through Bubble Tea commands so the interface stays responsive and cancellable where cancellation is safe.

Mouse clicks on fields, choices, tabs, rows, and confirmation buttons dispatch the same messages used by keyboard interaction; mouse handlers do not bypass business or security logic.

## Main areas

The persistent navigation covers the main operational surfaces:

```text
Workspaces  MCP  Tunnel  Requests  Logs  Config  Runtime
```

Additional resources such as workspace containers, managed tunnels, and About/build information are reachable through actions, Quick Open, or deep links.

The Logs page uses a natural-width `Runtime | Command Execution` tab list rather than an evenly divided navigation bar. `Runtime` loads persistent runtime history before opening its live stream and follows new events in real time. Its default visibility is `Verbose`, which includes useful lifecycle, approval, tunnel, and completed tool-call events while keeping debug diagnostics hidden. The Filters form can switch visibility between `Normal`, `Verbose`, and `Debug`; the selected visibility applies consistently to both journal history and live events.

`Command Execution` reuses the same bounded execution feed produced by the runtime for Admin UI command observability, across all registered workspaces. It replays the recent feed before continuing live and renders stdout and stderr as one combined stream in execution-event order instead of splitting them into separate panels. The view keeps at most 4000 feed events and supports follow/pause, reconnect, clear-view, keyboard scrolling, and mouse scrolling. Its contextual key hints stay pinned to the bottom even when the feed is empty. If an older running server does not expose the execution feed endpoint yet, the page reports `RESTART REQUIRED` instead of retrying forever.

The Command Center covers the public CLI capability inventory rather than mechanically copying Cobra into nested menus. Related commands are grouped around the resource they operate on.

## Scripting and automation

Do not automate the full-screen TUI. Use normal commands instead:

```bash
cgm workspace list
cgm workspace list --json
cgm mcp server list --json
cgm config verify
cgm status
```

Use `--json` or other structured-output flags where supported. Normal CLI commands remain the compatibility surface for scripts and pipelines; `cgm tui` is intentionally TTY-only.

The interaction model is explicit: interactive work starts with `cgm tui`, while normal `cgm ...` commands stay deterministic. Per-command interactive flags and automatic TUI branching are not part of the public CLI surface.

## Release and parity guarantees

The project keeps a canonical inventory of public CLI capabilities and verifies that every capability has a TUI representation discoverable through its CLI path. CI/release gates also exercise route parsing, Command Palette and Quick Open navigation, non-TTY refusal, and representative model integration without trying to drive a real interactive terminal session in CI.

The portable release smoke additionally checks `cgm tui --help`, the non-TTY refusal path, and normal CLI plain/JSON output so TUI evolution cannot silently break the scriptable interface.
