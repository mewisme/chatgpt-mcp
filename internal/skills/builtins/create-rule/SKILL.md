---
name: create-rule
description: Create persistent CGM rules for agent guidance. Use when the user wants coding standards, project conventions, always-on guidance, or file-specific rules in global/workspace CGM rule storage.
---

# Create a CGM rule

Infer the convention from the conversation and project. Decide `global` or `workspace` scope; ask only if it is genuinely ambiguous. Do not invent a destination path.

Decide always-on versus file-specific. File-specific rules need at least one concrete glob. Always-on rules must not include globs.

Synthesize concise, actionable content. Prefer one concern per rule. Include concrete examples when useful.

Then call `create_rule` with `workspace_id`, `scope`, `mode`, `name`, `description`, `always_apply`, `globs`, `content`, and `dry_run` when previewing.

Do not write the rule file with `write_file`, `apply_patch`, `edit_file`, `run_command`, or any other filesystem tool. Do not generate final YAML frontmatter; `create_rule` owns canonical serialization. Pass `scope`; the tool writes `.md` under native CGM roots.

If `create_rule` rejects the operation, report the error. Do not fall back to generic writes.
