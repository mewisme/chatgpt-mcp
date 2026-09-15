# Plugins

`chatgpt-mcp` plugins extend runtime behavior through signed, versioned artifacts without giving plugins authority over the core workspace or control-plane policy.

Plugins are local-user extensions, not a kernel sandbox. A native plugin process runs as the same operating-system user as `chatgpt-mcp`; use a VM/container, OS sandbox, or separate user identity when plugin code itself is not trusted.

## Operator workflow

Browse and inspect plugins with the CLI or the **Plugins** page in `cgm tui`:

```bash
cgm plugin search bash
cgm plugin info official/bash
cgm plugin list
cgm plugin outdated
```

Install and manage a plugin:

```bash
cgm plugin install official/bash
cgm plugin verify bash
cgm plugin disable bash
cgm plugin enable bash
cgm plugin update bash
cgm plugin rollback bash
cgm plugin prune bash
cgm plugin prune --retain 1 --cache
cgm plugin uninstall bash
```

`cgm plugin rollback <plugin> [version]` rolls back to a retained version. With no version it selects the newest retained version older than the active version. Rollback does not trust the retained executable blindly: the exact version is resolved through the configured signed registry again, the manifest and artifact are verified again, and the freshly verified payload replaces the retained copy before activation.

The default update retention policy keeps the active version plus the two newest inactive rollback versions. `cgm plugin prune [plugin] --retain N` applies the same retention rule manually; without a plugin it also removes orphaned inactive versions left by uninstalled plugins. Add `--cache` to remove registry/download cache and stale extraction directories without touching config, lock state, or the active payload.

## Registries and trust

The official registry is built in. Additional registries must be configured with an HTTPS URL and a pinned Sigstore identity:

```bash
cgm plugin registry add community https://plugins.example.com/releases \
  --issuer https://token.actions.githubusercontent.com \
  --repository example/plugins
```

A registry trust pin identifies the expected Sigstore OIDC issuer and signing repository. Installing a plugin verifies the trust chain before activation:

1. registry metadata is loaded from the configured registry;
2. signed registry metadata is checked against the pinned Sigstore identity;
3. the selected publisher must be trusted by that registry metadata;
4. the exact plugin manifest signature is verified;
5. the platform artifact SHA-256 must match the signed manifest;
6. the extracted manifest, publisher, platform, core compatibility, and capability dependencies are validated before activation.

A custom registry is never allowed to replace the built-in `official` registry. Unqualified plugin resolution is only used for registries explicitly configured to allow it.

## Desired state versus activation state

Plugin state is deliberately split:

- `plugins.json` contains portable **desired state**: registries and exact desired plugin versions plus enabled/disabled intent;
- `plugins.lock.json` contains machine-local **verified activation state**: active version, publisher, manifest digest, artifact digest, and enabled state;
- installed payloads live under the plugin data root;
- downloaded registry/artifact cache lives under the plugin cache root.

This split keeps backup/import portable without treating executable state from another machine as trusted.

`cgm config export` includes `plugins.json` but excludes `plugins.lock.json`, installed plugin payloads, and registry/download cache. `cgm config import` restores desired state and reports plugins that are missing, incompatible, or pending activation; it does not silently install executable payloads. An existing local lock is preserved during forced import.

## Windows Bash provider

On Windows the managed shell order is:

```text
configured Bash
→ Git for Windows Bash
→ official Bash plugin
```

If Git Bash is available, `chatgpt-mcp` uses it and automatically disables the installed `bash` plugin in local state. The plugin payload is retained but does not need to be re-enabled while Git Bash remains available. `C:\Windows\System32\bash.exe`/the WSL launcher is not treated as Git Bash.

If no usable Bash provider exists, diagnostics point to:

```bash
cgm plugin install bash
```

## Recovery and doctor

Run:

```bash
cgm doctor
```

Startup and `doctor` reconcile plugin activation state before capabilities are used. Reconciliation is fail-safe:

- a structurally corrupt lock file is quarantined as `plugins.lock.json.corrupt-<timestamp>` and replaced with an empty lock;
- desired state in `plugins.json` is preserved;
- an enabled lock entry with a missing payload, manifest/publisher mismatch, artifact digest mismatch, unsupported platform, or core incompatibility is disabled in the lock;
- if a provider becomes unavailable, enabled dependents that require its capability are also disabled;
- reconciliation never converts an untrusted payload into trusted activation state.

After recovery, explicitly repair/install the desired plugins and run `cgm plugin verify <id>` before relying on them again.

## Manifest authoring

A plugin artifact contains a strict `plugin.json` manifest. Unknown JSON fields are rejected. IDs, publishers, capability names, and platform names use canonical lowercase names; plugin versions are SemVer without a leading `v`.

Example command-wrapper manifest:

```json
{
  "schema": 1,
  "id": "rtk",
  "name": "RTK command wrapper",
  "publisher": "example",
  "version": "1.2.3",
  "type": "command-wrapper",
  "requires": { "chatgpt-mcp": ">=0.2.0" },
  "provides": ["command-wrapper/rtk"],
  "permissions": ["process/execute"],
  "platforms": {
    "linux/amd64": {
      "artifact": "rtk-1.2.3-linux-amd64.tar.gz",
      "sha256": "<64 lowercase hex characters>",
      "archive": "tar.gz",
      "entrypoint": "bin/rtk"
    }
  }
}
```

Recognized plugin types are `runtime`, `command-wrapper`, `hook`, `tool-provider`, `secret-provider`, and `formatter`. Recognized capability namespaces are `shell/*`, `command-wrapper/*`, `hook/*`, `tool-provider/*`, `secret-provider/*`, and `formatter/*`. A manifest must provide at least one capability.

Supported permissions are:

| Permission | Meaning |
| --- | --- |
| `process/execute` | plugin entrypoint may be invoked as a subprocess |
| `network/outbound` | declares outbound-network intent |
| `filesystem/plugin-data` | declares access to plugin-owned data |
| `filesystem/workspace-read` | declares workspace read intent |
| `filesystem/workspace-write` | declares workspace write intent |
| `hook/tool-observe` | required by observation hooks |
| `hook/tool-control` | required by pre-tool control hooks |

Permissions are declarations enforced by capability contracts where implemented; they do not create an OS sandbox around arbitrary native code.

## Shell capability

A `shell/bash` provider is a runtime plugin whose platform entrypoint is the Bash executable itself. The core selects it only when no configured/Git Bash provider has priority. Foreground and background shell execution use the same provider resolver.

## Hook capability

Implemented hook capabilities are:

```text
hook/pre-tool-use
hook/post-tool-use
hook/tool-error
hook/tool-denied
```

`hook/pre-tool-use` requires both `process/execute` and `hook/tool-control`. Observation hooks require `process/execute` and `hook/tool-observe`.

Hook subprocesses receive one bounded JSON event on stdin and return one bounded JSON result on stdout. The protocol schema is `1`. A pre-tool hook may return `continue`, `deny`, or `require_approval`; deny/approval decisions require a reason. Observation hooks may only continue.

Pre-tool hooks are fail-closed on timeout, execution failure, or invalid protocol output. Observation hooks are bounded asynchronous notifications and fail open; failures/drops are counted rather than blocking the tool result.

Hook execution carries provenance (`execution_id`, parent ID, origin, hook depth), and hook-originated work is not recursively dispatched back into hooks. Tool-call diagnostics record the pre-tool hook provider ID, version, and capability when a plugin participates in the decision. The subprocess environment is allowlisted and marked as tool context; approval/control-plane capability is not inherited.

Hooks can add policy, but they cannot remove core policy. Core workspace containment, control guard, and approval checks still run independently.

## Command-wrapper capability

A wrapper capability such as `command-wrapper/rtk` requires `process/execute`. The entrypoint basename must match the capability suffix (`rtk` or `rtk.exe`).

The schema-1 subprocess protocol uses three operations:

```text
can_wrap
rewrite
security_projection
```

For each operation the core sends:

```json
{
  "schema": 1,
  "operation": "can_wrap",
  "tool": "run_command",
  "command": "git push --force origin main"
}
```

`can_wrap` returns `{ "schema": 1, "can_wrap": true|false }`. `rewrite` and `security_projection` return `{ "schema": 1, "command": "..." }`.

The current wrapper contract is intentionally strict. Exactly one wrapper may apply. A rewrite must be a transparent executable prefix (`rtk <requested command>`), and the security projection must equal the original requested command exactly. The core classifies the security projection after rewriting, so a wrapped operation such as:

```text
git push --force origin main
→ rtk git push --force origin main
```

still requires the same destructive approval as the unwrapped command. A plugin cannot return a harmless projection to hide a dangerous operation.

Wrapper subprocesses have bounded input/output, a timeout, an allowlisted environment, and no inherited approval capability.

## Security invariants

Plugin capabilities are subordinate to core policy:

- plugins cannot disable the control guard or manufacture approvals;
- plugin rewrites cannot weaken command classification;
- plugins cannot expand registered workspace roots by returning different paths;
- plugin installation/trust configuration is a local operator action, not an Agent self-grant path;
- signed metadata and SHA-256 verification happen before payload activation;
- activation records bind registry, publisher, manifest digest, artifact digest, version, and enabled state;
- corrupt or unverifiable activation state is disabled instead of being guessed/reconstructed from executable files;
- plugin subprocesses receive a reduced environment and do not inherit control-plane approval authority;
- trace, logger, and persisted runtime-event paths apply shared secret redaction before diagnostic data is emitted or stored.

These rules protect the `chatgpt-mcp` application boundary. They do not make arbitrary same-user native plugin code harmless; use OS isolation for that threat model.
