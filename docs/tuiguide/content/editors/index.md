# Editors & Forms

Data-entry workflows use full-page editors instead of modal forms. Dialogs are reserved for confirmation or operation progress. This keeps long forms usable in small terminals and gives every editor the same navigation, validation, and dirty-state behavior.

## Saving and leaving

Mutating editors use `Ctrl+S` for their explicit primary action, such as `save`, `create`, `authorize`, `install`, or `update`. Reaching the final field or pressing `Enter` does not persist a mutation.

Non-mutating editors can opt into natural completion. In those editors, `Enter` advances fields/sections and invokes the primary action only when the final visible field completes. Runtime Logs filters, Command Execution scope, and Workspace Project Context build use this mode. Multiline text still owns `Enter` for newlines and never auto-submits from a newline.

When a save succeeds, the current draft is committed as the editor baseline before navigation begins. This is important because navigation is protected by the global dirty-draft guard. A successful save can therefore show its success toast and return to the parent without incorrectly asking to discard the data that was just saved.

If a save fails, the editor stays open and preserves the exact draft. Validation/backend errors are rendered within the editor. Secrets are not echoed into generic error text or toast content.

## Dirty drafts

Changing a field makes the editor dirty. Attempting to navigate away from a dirty editor opens **Discard changes?**. Choose Discard to continue navigation or Keep editing to remain on the current editor. A submitting editor blocks navigation until its active operation has completed or been safely cancelled.

## Sections

Long editors are divided into visible section tabs, for example `General / Connection / Authentication / Tools`. `Tab` and `Shift+Tab` move through fields and across section boundaries. Mouse clicks on section titles use the same section-switch messages.

Section titles use one shared visual contract: no title background, consistent title/meta typography, a divider, and a stable body origin. This is the same style used by Instruction's Global Context section.

## Inputs and labels

Labels describe what a value represents. Input-specific entry guidance is placed in the input placeholder rather than a separate sublabel where possible. This keeps forms compact while still showing format hints such as RFC3339 timestamps, comma-separated lists, optional values, or blank-to-preserve-secret behavior.

Multiline text uses Bubbles textarea semantics. `Enter` inserts a newline instead of submitting. JSON creation mode and Global Context editing therefore behave like text editors rather than single-line forms.

## Boolean values

Persistent booleans use a two-state Switch control. `Space` or a mouse click toggles the value. `Enter` remains normal field traversal; only an editor explicitly configured for non-mutating completion can turn final-field traversal into its primary action.

## File and directory paths

Path-aware fields are picker-first where the target is expected to exist. `Ctrl+O` switches between the Bubbles file picker and manual text entry. Both modes share the same bound value, so changing modes does not lose the current path draft.

The field can enforce file-vs-directory expectations, workspace/root boundaries, relative output, and allow-missing behavior when creating a new destination. Validation happens at the same editor boundary used for normal form fields.
