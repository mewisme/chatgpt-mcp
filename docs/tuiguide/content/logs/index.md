# Logs

Logs contains `Runtime / Command Execution` tabs. Both use the shared SectionLayout so titles and browser/help geometry stay consistent and local help remains directly above the application footer.

## Runtime logs

Runtime loads persisted journal history and then follows live events. The browser supports normal list navigation/filtering and can open a structured event detail. Refresh reconnects history/live state without changing the overall page layout.

The Filter editor uses `Range / Filters` sections. It controls time range and structured criteria such as visibility/level, component, session, workspace, tool, request, and related event metadata where available. `Enter` advances through the editor and applies the filter when the final visible field completes. Time strings accept the formats described by their field labels/placeholders.

Filter validation happens before replacing the active query. An invalid filter draft stays open with feedback instead of partially changing the visible stream.

## Visibility

Runtime log visibility can distinguish normal operational events, verbose lifecycle/approval/tool-call events, and debug diagnostics. The selected visibility applies consistently to history and the live stream.

## Command Execution

Command Execution displays the bounded globally ordered execution feed produced by the runtime. It replays recent execution events and continues live, preserving stdout/stderr ordering from the event stream rather than rendering independent panels that lose interleaving. Each visible segment is one block: `START` opens the first segment, `RUNNING` marks the trailing segment that is still live, `PAUSE` is used only when another execution actually interrupts it, `CONTINUE` opens a resumed execution, and `END` appears only after completion. The execution ID is an `Execution` field inside the block without an `exec_id=` prefix. Metadata, command/output, and footer state are separated by two horizontal borders; a segment with no command/output content shows `No output`. Frames and wrapped content fit the current viewport width and reflow after terminal resize.

Press `f` to choose `combined`, one workspace, or one workspace-container scope. Workspace scope then asks whether to `Run commands` or `View process`. `View process` lists managed background jobs for the selected workspace and attaches to the chosen process using its execution ID. Process output stays on the same global execution stream; no extra process SSE connection is created. Normal command views exclude `start_process` executions, while process view shows only the selected process.

Complete the final visible selector with `Enter` to apply it. If a selected process finishes while it remains open, its final output and `END` block stay visible. Leaving a finished process view deletes only that finished process record. Leaving a running process view only detaches the TUI; it never sends a stop signal. Scope changes filter the existing global feed locally and do not reconnect it. Container scope resolves current member workspaces without granting execution permission. Reconnect or reapply the scope after container membership changes. If a selected workspace/container disappears, the view reports it as unavailable instead of silently changing scope.

The page supports follow/pause, reconnect, clear-view, keyboard scrolling, and mouse scrolling. Local help is pinned to the section footer even when the feed is empty.

When you leave Logs for another top-level page and return during the same TUI process, Logs restores its last stable tab, applied Runtime filters/visibility, execution scope, follow/pause state, Runtime selection when still present, and Command Execution scroll offset where the current bounded snapshot allows it. A selected process is restored only while it is still considered running; finished process selections are dropped as part of cleanup. The page opens fresh streams on return; event buffers, SSE objects, editors, progress, and errors are not cached. Exiting the TUI clears this in-memory view state. Mutation/editor forms are not restored as last views.

If the running runtime is too old to expose the execution feed, the page reports that a restart is required rather than retrying indefinitely.

## Layout guarantees

Long structured fields and event content wrap to the page width. Browser content is clamped to its assigned body height, and browser help is rendered externally by SectionLayout so Bubbles does not reserve a second hidden footer area.
