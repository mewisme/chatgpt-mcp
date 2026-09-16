# Workspaces

A workspace is the concrete project boundary that `chatgpt-mcp` uses for filesystem, shell, Git, process, project-context, memory, rule, skill, and checkpoint operations.

The important distinction is:

```text
ws_*  = concrete execution/filesystem workspace
wsc_* = logical orchestration group of workspaces
```

A workspace container never becomes a filesystem permission boundary and never replaces a concrete `ws_*` target.

## Register a workspace

```bash
cgm workspace register ~/projects/my-project
```

Inspect it:

```bash
cgm workspace list
cgm workspace show ws_...
```

Workspace identity lives in `<workspace>/.cgm/workspace.json` and is stable across register, restart, unregister/re-register, and relocate. Older path-derived IDs may remain usable as aliases after migration.

## Effective filesystem scope

A workspace can reach:

```text
registered workspace root
+ global permissions.allow_dirs
+ workspace-specific access directories
```

Add a narrow workspace-specific directory when a project genuinely needs files outside its root:

```bash
cgm workspace access add ws_... /path/to/build-cache
cgm workspace access list ws_...
cgm workspace access remove ws_... /path/to/build-cache
```

Global extra roots apply to every workspace and should be used more carefully:

```bash
cgm config set permissions.allow_dirs /path/one,/path/two
```

Paths are canonicalized and symlink escapes are rejected. Read [Security](security.md#workspace-boundary) for the full boundary.

## Explicit targeting

A single MCP session may work with multiple registered projects. Each workspace-scoped operation still names the intended `workspace_id`; the runtime does not maintain a hidden global “current workspace” and does not silently fall back to another project.

That means an Agent can move between projects safely by targeting different `ws_*` IDs while workspace-specific state remains isolated.

## What stays isolated

Switching between workspaces does not merge their:

- filesystem roots and extra access directories
- shell working directory and process state
- Git working context
- project instructions and context
- workspace memory
- rules and skills
- checkpoints/rewind state
- approval state
- workspace-scoped plugins and their config/payloads

This isolation is why concrete `ws_*` targets remain required even when several projects belong to the same workspace container.

## Local `.cgm` state

Each registered workspace owns persistent state under `<workspace>/.cgm`:

```text
.cgm/workspace.json   stable workspace identity
.cgm/config.json      workspace-specific access and aliases
.cgm/state/           shell/REPL and other workspace-owned state
.cgm/memory/          workspace memory
.cgm/checkpoints/     rewind snapshots
.cgm/plugins/         workspace-scoped plugin desired/lock/config/payloads
.cgm/cache/           rebuildable cache
.cgm/runtime/lock     OS-backed exclusive runtime lock
```

Workspace plugin desired state is `.cgm/plugins/desired.json`, lock state is `.cgm/plugins/lock.json`, per-plugin settings are `.cgm/plugins/config/<id>.json`, and payloads are `.cgm/plugins/data`. Plugin registries and the download cache stay global. Relocate keeps plugin state because `.cgm` moves with the project directory. `workspace purge` deletes `.cgm`, including workspace plugins. Unregister unloads runtime plugin stores but leaves `.cgm` on disk.

The global `workspaces.json` index stores only identity/path pointers. Registering a workspace creates `.cgm` when it is missing and reuses an existing identity instead of minting a new ID. Leftover global `workspaces/<id>` state is copied into `.cgm` on load: missing files are added, checkpoint indexes are merged by ID, existing local files win on other conflicts, and the leftover directory is then removed.

For Git checkouts, CGM adds the correct rooted `.cgm/` pattern to Git's `info/exclude`. It does not create `.cgm/.gitignore` and does not edit project `.gitignore`.

Public CGM filesystem tools and clearly destructive shell commands cannot mutate an active workspace's `.cgm` directory. That protection applies to CGM operations only; an external `rm -rf .cgm` is still possible on the host.

## Relocate a moved project

If the project directory has already been renamed or moved, relocate the existing workspace instead of registering the destination as an unrelated project:

```bash
cgm workspace relocate ws_... /new/path/to/project
```

Relocation updates the trusted root and keeps the existing workspace ID. Workspace-scoped persistent state stays with the project directory, and container membership is preserved.

Relocate does **not** move project files. It is a trusted local control-plane operation available through CLI, TUI, and Admin surfaces rather than an Agent filesystem tool.

## Workspace containers

Containers group registered workspaces for orchestration:

```bash
cgm workspace container list
cgm workspace container create "Backend projects"
cgm workspace container add wsc_... ws_... ws_...
cgm workspace container show wsc_...
```

A `wsc_*` ID answers “which workspaces belong together?”, not “which filesystem may this tool access?”.

Agent-facing container tools can discover the members of a group, but substantial work still targets one or more concrete member `ws_*` IDs individually. Resolving a container does not merge member context, memory, permissions, shell state, or checkpoints.

## Runtime synchronization

Workspace registry changes made through supported control surfaces are synchronized with a running runtime so subsequent workspace reads can see the new registry state without a full runtime restart.

If an operation reports a synchronization failure, inspect runtime status/logs before assuming the live process adopted the change.

## Recommended practice

- Register only project roots ChatGPT actually needs.
- Prefer workspace-specific extra directories over broad global roots.
- Keep different trust domains in separate workspaces or separate runtime instances where appropriate.
- Use containers for grouping/orchestration, never as permission shortcuts.
- Relocate an existing workspace after a project moves instead of registering a duplicate.

## Unregister vs purge

`unregister` removes the workspace from the local index and releases its runtime lock. Project files and `.cgm` stay on disk, so a later register of the same directory restores the same identity and state.

Delete local state only with an explicit destructive command:

```bash
cgm workspace purge ws_... --confirm
cgm workspace purge ~/projects/copied-project --confirm
```

`purge` unregisters the workspace if needed, then deletes `<workspace>/.cgm`. Use it to reinitialize a copied project that duplicated another workspace's identity. It refuses while another CGM runtime still holds that workspace's lock.

## Unavailable workspaces

A missing directory or corrupt `.cgm` identity stays in the index. `workspace list` and `workspace show` report it as unavailable, sibling workspaces still load, and CGM does not mint a replacement ID. Unregister or purge the entry, or relocate it after the project directory is restored.

See [Security](security.md) for the trust model and [Configuration](configuration.md) for persistent access settings.
