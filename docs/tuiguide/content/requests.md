# Requests & Approvals

Requests is the persistent approval inbox. The live approval dialog is a separate global surface for newly pending control requests, but both use the same approval state and security semantics.

## Request tabs

The page provides `Pending / History / All` views. Pending contains unresolved, non-expired requests. History contains resolved or expired requests. All combines both. Create Test opens a synthetic request editor useful for validating approval UX without needing a real protected command.

## Request detail

Open a request to see its title, workspace, tool, source, guard information, timestamps, status, and countdown. Child pages expose long Arguments and Guard content without cramming everything into the overview.

Detail content uses a viewport. Periodic countdown refreshes preserve the viewport offset, so scrolling down does not jump back to the top every second.

## Approve and deny editors

Approve/Deny are routed editors so an optional reason can be entered without a modal form. Immediately before mutation the page fetches the request again. If another client has already resolved it, or it expired while the editor was open, the mutation is not sent and the editor keeps its reason draft with a stale-state error.

This re-check prevents a visually stale approval editor from resolving a request whose security state has already changed.

## Live approval dialog

When a pending approval arrives, the global dialog shows request metadata and arguments. The dialog is width- and height-bounded to the terminal. Its content is in a scrollable Bubbles viewport while Approve/Deny buttons remain fixed below the viewport, so long arguments cannot push the controls off-screen.

Use `j/k`, arrows, `PgUp/PgDn`, or the mouse wheel to scroll. `a` approves, `d` denies, and left/right changes the selected confirmation button. Countdown refreshes update content without resetting the current scroll offset.

The dialog can widen up to its responsive cap, but always stays inside the terminal with margins.