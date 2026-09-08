# Shell & Execution

Shell configuration controls command execution policy independently from workspace filesystem boundaries and the approval/control-guard layer.

## Approval policy

The shell approval policy determines when a command requires user approval. Changing approval behavior does not grant filesystem access outside the workspace or configured allowed directories; those checks remain separate.

## Allow and deny commands

Allow/deny command patterns are edited as multiline content in a full-page editor. `Ctrl+S` validates and persists the draft. After a successful save, the editor accepts the persisted value as its new clean baseline before navigation, so closing the success toast does not trigger a false discard prompt.

Failed saves keep the exact draft in place and surface validation or persistence errors in the editor.

## Security layers

Treat command policy, workspace access, runtime control guard, and approval requests as distinct layers. Relaxing one layer does not implicitly relax the others.
