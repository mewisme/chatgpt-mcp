# Request Editor Fields

## Create Test Request

This editor creates a synthetic approval request for testing the Requests TUI and approval workflow. It does not execute the displayed command.

### Workspace ID

Synthetic workspace label attached to the test request. The default draft is `ws_dummy`. It is metadata for exercising request presentation/filtering and does not cause a workspace command to run.

### Title

Human-readable approval title shown for the synthetic request. The default draft is `Allow test command`.

### Command

Command text displayed in the synthetic approval request. It is deliberately display-only and is **not executed** by creating the test request. The default is `echo test approval`.

## Approve/Deny Request

### Reason

Optional human-readable reason attached to the resolution. The same field is used for approve and deny editors.

Before mutation, the TUI re-fetches the request and verifies that it is still pending and not expired. If another actor already resolved it or it expires while the editor is open, no resolve mutation is sent; the editor/draft stays open with stale-state feedback.

## Live approval dialog

The live approval dialog is separate from these routed editors. Request details/arguments are placed in a bounded scrollable viewport; Allow/Deny controls remain outside the viewport so long content cannot push the action buttons off-screen. The countdown refresh preserves viewport scroll position for the same request.
