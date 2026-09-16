---
name: create-skill
description: Create or update reusable CGM skills. Use when the user wants to create a skill, define a repeatable agent workflow, add SKILL.md instructions, or manage a global/workspace CGM skill.
---

# Create a CGM skill

Gather the workflow, triggers, and intended `global` or `workspace` scope from the conversation. Ask about scope only when it is genuinely ambiguous. Do not invent a destination path.

Choose a canonical name: lowercase, max 64 characters, matching `^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`.

Write a description that states WHAT the skill does and WHEN to use it. Include trigger terms.

Synthesize concise instructions. Prefer under 500 lines. Put long references, scripts, and assets in supporting files. Preserve verbatim user-provided instruction text when requested. Progressive disclosure: keep SKILL.md short and link out.

If updating, inspect the existing skill so unlisted supporting files stay unless they must be removed.

Then call `create_skill` with `workspace_id`, `scope`, `mode`, `name`, `description`, `instructions`, optional `files` / `remove_files`, and `dry_run` when previewing.

Do not write SKILL.md or supporting files with `write_file`, `write_file_base64`, `apply_patch`, `edit_file`, `run_command`, or any other filesystem tool. Pass `scope`; `create_skill` resolves the path.

If `create_skill` rejects the operation, report the error. Do not fall back to generic writes.
