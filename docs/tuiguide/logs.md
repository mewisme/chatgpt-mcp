# Logs

Logs contains `Runtime / Command Execution` tabs. Both use the shared SectionLayout so titles and browser/help geometry stay consistent and local help remains directly above the application footer.

## Runtime logs

Runtime loads persisted journal history and then follows live events. The browser supports normal list navigation/filtering and can open a structured event detail. Refresh reconnects history/live state without changing the overall page layout.

The Filter editor uses `Range / Filters` sections. It controls time range and structured criteria such as visibility/level, component, session, workspace, tool, request, and related event metadata where available. Time strings accept the formats described by their field labels/placeholders.

Filter validation happens before replacing the active query. An invalid filter draft stays open with feedback instead of partially changing the visible stream.

## Visibility

Runtime log visibility can distinguish normal operational events, verbose lifecycle/approval/tool-call events, and debug diagnostics. The selected visibility applies consistently to history and the live stream.

## Command Execution

Command Execution displays the bounded execution feed produced by the runtime. It replays recent execution events and continues live, preserving stdout/stderr ordering from the event stream rather than rendering independent panels that lose interleaving.

The page supports follow/pause, reconnect, clear-view, keyboard scrolling, and mouse scrolling. Local help is pinned to the section footer even when the feed is empty.

If the running runtime is too old to expose the execution feed, the page reports that a restart is required rather than retrying indefinitely.

## Layout guarantees

Long structured fields and event content wrap to the page width. Browser content is clamped to its assigned body height, and browser help is rendered externally by SectionLayout so Bubbles does not reserve a second hidden footer area.