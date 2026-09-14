# Logs

Logs has three tabs:

```text
Runtime | Command Execution | Tool Calls
```

Each tab can be shown as a **Browser** or **Timeline**. Press `v` to switch view and `m` to change Stream Mode.

## Follow

`Space` pauses or resumes follow in either view. While following, Browser stays on the newest visible record and Timeline stays at the tail.

Moving Browser selection away from the newest record pauses follow so new events do not steal the selection. Press `Space` to return to the tail.

Leaving Logs for another top-level page closes the live feeds. Returning rebuilds Logs from fresh history/snapshots and resumes follow automatically.

## Runtime

Runtime shows persistent runtime journal history and continues with live runtime events. Tool-call lifecycle records are kept in the dedicated Tool Calls tab rather than duplicated into Runtime.

Use the filter editor for time range, visibility, component, session, workspace, tool, status, source, event glob, and text matching.

## Command Execution

Command Execution shows the runtime execution feed with stdout/stderr preserved in stream order.

Stream Mode can show combined activity, a workspace, a workspace container, or a managed process for a selected workspace. Process view follows the selected background process without creating a separate process-output stream.

Browser is useful for selecting one execution. Timeline is useful for watching command activity as it happens.

## Tool Calls

Tool Calls shows authoritative tool-call lifecycle records.

Browser detail renders the complete structured call body. Timeline renders request/result/error blocks in chronological order.

Tool Calls supports the normal combined/workspace/container stream scopes but does not expose Command Execution's Process mode.

## Reconnect and clear

Use `r` where available to reconnect/refresh the active stream. Command Execution can clear its current view without deleting persisted runtime history.

Exact page-local bindings are always shown above the application footer.
