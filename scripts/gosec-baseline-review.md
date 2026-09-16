# Gosec baseline review (2026-09-16)

Reviewed with gosec v2.29.0, using the CI exclusion for
`plugins/cf-tunnel/internal/cloudflared`. A matching baseline is not a claim
that the project has no vulnerabilities. Re-review these exceptions if their
inputs or trust boundaries change; do not disable whole rules.

## Fixed before refreshing

- G122 and G304 in `internal/nativeinstruction/author.go`: skill support files
  are now read through an open `os.Root`, with opened-file identity checks and
  bounded reads. Regression tests cover replacement files, escaping symlinks,
  root replacement, and file/tree size limits.
- Three G104 findings in `internal/workspace/runtime_lock.go`: rollback errors
  are joined with the triggering error instead of discarded.
- Removed two stale G304 entries for `internal/plugin/mutation_lock.go` and
  `internal/workspace/workspace.go`.

## Approved exceptions: 42 findings

After explicit user approval, the baseline was refreshed to accept these
42 findings and remove the two stale entries (132 total baselined findings).
Exceptions are scoped to individual code fingerprints, not whole rules.
This accepts the documented risks; it does not eliminate them.

| Rule | Count | Locations and rationale |
| --- | ---: | --- |
| G204 | 12 | `application/tui.go` and `runtimeplugin/session.go` intentionally execute installed plugin entrypoints. `notification/host.go` launches platform helpers using argv; notification text is passed as data, not interpolated into scripts. `licenseinventory/{archive,inventory}.go`, `pluginbuild/{build,compress}.go`, and `plugindev/{bootstrap,fingerprint}.go` run developer-controlled build tools with argv. These paths require trusted installed plugins, source trees, build configuration, and local toolchains. They are not sandboxes for untrusted build code. |
| G301 | 4 | `licenseinventory/inventory.go` and `pluginbuild/build.go` create public release-output/staging directories as 0755. They hold distributable binaries, manifests, and license text, not runtime secrets. |
| G306 | 6 | `licenseinventory/inventory.go` writes four public license/SBOM outputs as 0644; `pluginbuild/build.go` writes a distributable manifest as 0644. `testutil/plugin.go` writes an executable fixture as 0700 inside `t.TempDir()`. |
| G304 | 3 | `licenseinventory/inventory.go` reads policy and license material from developer-selected source/dependency directories. |
| G304 | 2 | `pluginbuild/build.go` reads build templates and generated archive input in the trusted release-build workflow. |
| G304 | 5 | `plugindev/{bootstrap,cache,context,fingerprint,workflow}.go` reads local build artifacts, cache markers, module identity, source hashes, and workflow definitions. Dev mode intentionally trusts the selected repository and toolchain. |
| G304 | 1 | `notification/host.go` is a file-reading adapter; its production caller supplies the fixed `/proc/sys/kernel/osrelease` path. |
| G304 | 1 | `oslock/lock.go` opens caller-selected lock files with 0600 permissions under application/workspace state directories. |
| G304 | 4 | `plugin/{config,pluginconfig,registry_client,resources}.go` reads local configuration, validated-ID config paths, safe-named registry sidecars, and checked regular skill metadata within a plugin payload. Local configuration/cache and installed payload ownership remain trust assumptions; these checks do not establish protection against a hostile same-user process mutating those directories. |
| G304 | 1 | `shell/session.go` reads a state path obtained from the workspace manager; persisted CWD is resolved again before use. No shell environment behavior was changed in this review. |
| G304 | 3 | `workspacestate/state.go` reads workspace-owned identity/config and Git's resolved exclude path. Worktrees may place Git metadata outside the worktree; restricting these reads to the worktree would break that supported layout. |

The separate SOCKS half-close regression is in the vendored-tree exclusion;
its bounded drain and response-preservation tests must be run independently
of the gosec baseline check.
