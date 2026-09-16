# Plugins

`chatgpt-mcp` plugins extend runtime behavior through signed, versioned plugin metadata and, where needed, packaged artifacts without giving plugins authority over the core workspace or control-plane policy.

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
cgm plugin install official/bash --scope global
cgm plugin install official/rtk --scope workspace --workspace ws_...
cgm plugin verify bash
cgm plugin disable bash
cgm plugin enable bash --scope workspace --workspace ws_...
cgm plugin update bash
cgm plugin rollback bash
cgm plugin prune bash
cgm plugin prune --retain 1 --cache
cgm plugin uninstall bash
```

`--workspace` implies `--scope workspace`. `--scope global --workspace ...` is rejected. Plugins that allow only one scope install there automatically. Schema-1 manifests and existing installs stay global; there is no automatic move into a workspace. A plugin that allows both scopes requires `--scope` or `--workspace` when stdin is not a TTY.

The official Admin UI is a platform-independent static plugin and is global-only:

```bash
cgm plugin install admin-ui
cgm plugin verify admin-ui
```

The core keeps the admin listener, authentication, `/api/*`, OAuth callback, activity endpoints, and security headers. `admin-ui` only provides signed static assets through `web-ui/admin` and requests no runtime permissions. Without an enabled provider, API routes remain available while the root UI returns a service-unavailable response with the install command. Auth and tunnel behavior stay in core; they are not workspace plugins.

The official RTK wrapper is host-backed. If RTK is not available on `PATH`, installation can use the manifest-declared verified portable binary or a supported global installer; manual shell installation hints remain recommendations only. `bash` and `rtk` may be installed globally or for one workspace. Built-in Ponytail and Caveman remain global-only.

`cgm plugin rollback <plugin> [version]` rolls back to a retained version. With no version it selects the newest retained version older than the active version. Current installs persist verified registry/publisher identity plus manifest and artifact digests alongside a local payload integrity root, so a retained version can be re-verified and activated while its registry is offline. Legacy retained versions created before that metadata existed fall back to registry re-resolution and signature verification before activation.

The default update retention policy keeps the active version plus the two newest inactive rollback versions. `cgm plugin prune [plugin] --retain N` applies the same retention rule manually; without a plugin it also removes orphaned inactive versions left by uninstalled plugins. Add `--cache` to remove registry/download cache and stale extraction directories without touching config, lock state, or the active payload. Update, rollback, prune, and verify act only on the selected scope's store.

## Install scope

Plugins install in exactly one of:

```text
global     once for this CGM install; visible to every workspace that uses that plugin type
workspace  one registered workspace; active only while operating in that workspace
```

Manifest `scopes` (schema 2) lists allowed values. Schema 1 has no field and is global-only. CLI/TUI/Admin cannot install a plugin into a scope the manifest forbids.

The same plugin ID cannot be enabled globally and in a workspace at the same time. Disabled copies may exist in both stores. Do not merge global and workspace config for the same ID.

Effective plugins for a request:

```text
no workspace context     global plugins only
workspace ws_...         global plugins + that workspace's plugins
```

Workspace plugins may depend on global providers. Global plugins may not depend on a workspace plugin.

`cgm plugin config` reads and writes global plugin settings. Workspace plugin settings live under that workspace's `.cgm` and are edited from TUI/Admin with an explicit workspace selected.

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
5. packaged plugins require the platform artifact SHA-256 to match the signed manifest; host-backed plugins require the manifest-declared host prerequisite preflight to pass;
6. the manifest, publisher, platform, core compatibility, and capability dependencies are validated before activation.

A custom registry is never allowed to replace the built-in `official` registry. Unqualified plugin resolution is only used for registries explicitly configured to allow it.

## Third-party registry authoring

A registry can be any HTTPS static asset root that follows the signed registry protocol; it does not need to share the ChatGPT MCP repository layout. A minimal registry contains:

```text
index.json
index.json.sigstore.json
publishers.json
publishers.json.sigstore.json
example-1.2.3.json
example-1.2.3.json.sigstore.json
example-1.2.3-linux-amd64.tar.gz
example-1.2.3-linux-amd64.tar.gz.sigstore.json
```

`index.json` and `publishers.json` are mutable discovery metadata and may be replaced after being re-signed. Versioned manifests and packaged artifacts are immutable: publish a new plugin version instead of replacing bytes behind an existing versioned filename. The registry index maps each plugin/version to its exact manifest filename, while the publisher index names the trusted publisher and its Sigstore issuer/repository identity.

Sign registry metadata, exact-version manifests, and packaged artifacts with Sigstore/cosign bundles. The client pins the registry signing identity when it is configured, verifies the root metadata first, then requires the selected plugin publisher to be trusted before accepting its manifest. A packaged artifact SHA-256 must be covered by the signed manifest. For portable host dependencies, prefer an exact upstream release URL plus a direct SHA-256 in the signed manifest; do not use moving aliases such as `latest` for an immutable plugin version.

Register the hosted registry with the same identity used by its signing workflow:

```bash
cgm plugin registry add community https://plugins.example.com/releases \
  --issuer https://token.actions.githubusercontent.com \
  --repository example/plugins
```

Without unqualified resolution, install with an explicit registry prefix such as `cgm plugin install community/example`. Enable unqualified resolution only when ambiguity with the official or another configured registry is acceptable; ambiguous names fail rather than being selected by registry order.

## Desired state versus activation state

Plugin state is deliberately split:

- global `plugins.json` contains portable **desired state**: registries and exact desired plugin versions plus enabled/disabled intent;
- global `plugins.lock.json` contains machine-local **verified activation state**: active version, publisher, manifest digest, platform integrity digest, and enabled state;
- workspace installs store the same desired/lock split under `<workspace>/.cgm/plugins/{desired.json,lock.json}` and payloads under `.cgm/plugins/data`;
- workspace plugin settings live under `.cgm/plugins/config/<id>.json`;
- registries remain global even for workspace installs; workspace desired files never contain registry definitions;
- installed version directories retain verified source metadata plus a payload-tree integrity digest; packaged payloads live under the selected scope's plugin data root, while global host-backed plugins retain metadata without copying the host executable;
- downloaded registry/artifact cache lives under the shared global plugin cache root.

Existing installs stay in the global files above. Upgrade does not infer workspace scope from the current directory.

`cgm config export` includes global `plugins.json` but excludes `plugins.lock.json`, installed plugin payloads, and registry/download cache. Workspace `.cgm/plugins` is workspace-owned state, not part of that bundle. `cgm config import` restores desired state and reports plugins that are missing, incompatible, or pending activation; it does not silently install executable payloads. An existing local lock is preserved during forced import.

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

Startup and `doctor` reconcile plugin activation state before capabilities are used. Global desired/lock files and each workspace `.cgm/plugins` store reconcile independently. Reconciliation is fail-safe:

- a structurally corrupt lock file is quarantined as `plugins.lock.json.corrupt-<timestamp>` and replaced with an empty lock;
- desired state in `plugins.json` is preserved;
- an enabled lock entry with a missing packaged payload, unavailable/invalid host prerequisite, manifest/publisher mismatch, platform integrity mismatch, unsupported platform, or core incompatibility is disabled in the lock;
- if a provider becomes unavailable, enabled dependents that require its capability are also disabled;
- reconciliation never converts an untrusted payload into trusted activation state.

After recovery, explicitly repair/install the desired plugins and run `cgm plugin verify <id>` before relying on them again.

## Manifest authoring

A plugin uses a strict `plugin.json` manifest. Unknown JSON fields are rejected. IDs, publishers, capability names, and platform names use canonical lowercase names; plugin versions are SemVer without a leading `v`. Most plugins ship a platform artifact, but a command-wrapper may instead declare a verified host executable and install only signed metadata.

Schema 1 has no `scopes` field and installs globally only. Schema 2 requires a non-empty unique `scopes` list of `global` and/or `workspace`. Do not add `scopes` to a schema-1 manifest.

Example packaged command-wrapper manifest:

```json
{
  "schema": 2,
  "id": "example-wrapper",
  "name": "Example Wrapper",
  "publisher": "example",
  "version": "1.2.3",
  "type": "command-wrapper",
  "scopes": ["global", "workspace"],
  "requires": { "chatgpt-mcp": ">=0.2.0" },
  "provides": ["command-wrapper/example-wrapper"],
  "permissions": ["process/execute"],
  "platforms": {
    "linux/amd64": {
      "artifact": "example-wrapper-1.2.3-linux-amd64.tar.gz",
      "sha256": "<64 lowercase hex characters>",
      "archive": "tar.gz",
      "entrypoint": "bin/example-wrapper"
    }
  }
}
```

A host-backed wrapper declares a generic host contract instead of an artifact. The plugin owns prerequisite checks, installation hints, and rewrite behavior; core only validates and executes the declared contract. For example:

```json
{
  "linux/amd64": {
    "host": {
      "executable": "example-wrapper",
      "checks": [
        { "name": "identity", "args": ["check"], "stdout_contains": "ready" }
      ],
      "install": [
        { "label": "Package manager", "command": "pkg install example-wrapper" }
      ],
      "command_wrapper": {
        "args": ["rewrite", "{command}"],
        "rewrite_exit_codes": [0],
        "passthrough_exit_codes": [1]
      }
    }
  }
}
```

Host-backed entries cannot mix `host` with `artifact`, `sha256`, `archive`, or `entrypoint` fields. Checks and install commands are declarative metadata. Manual shell hints are recommendations only; a structured global installer is executed only after the operator explicitly selects that option.

Recognized plugin types are `runtime`, `command-wrapper`, `hook`, `tool-provider`, `secret-provider`, `formatter`, and `web-ui`. Recognized capability namespaces are `shell/*`, `command-wrapper/*`, `hook/*`, `tool-provider/*`, `secret-provider/*`, `formatter/*`, and `web-ui/*`. A manifest must provide at least one capability. A `web-ui` plugin must provide exactly one `web-ui/*` capability, requests no runtime permissions, and may use `any/any` for a platform-independent static bundle.

Supported permissions are:

| Permission | Meaning |
| --- | --- |
| `process/execute` | plugin entrypoint or declared host executable may be invoked as a subprocess |
| `network/outbound` | declares outbound-network intent |
| `filesystem/plugin-data` | declares access to plugin-owned data |
| `filesystem/workspace-read` | declares workspace read intent |
| `filesystem/workspace-write` | declares workspace write intent |
| `hook/tool-observe` | required by observation hooks |
| `hook/tool-control` | required by pre-tool control hooks |

Permissions are declarations enforced by capability contracts where implemented; they do not create an OS sandbox around arbitrary native code.

## Official plugin CI definitions

Official marketplace CI is definition-driven. `plugins/workflow.json` is the only plugin list consumed by `.github/workflows/plugins.yml`; build, publish, signing, immutable-asset checks, and post-publish smoke jobs are generic matrices.

Adding another official plugin does not require adding or editing GitHub Actions jobs. Add the normal `plugins/<id>/plugin.json` and registry entry, then add one definition to `plugins/workflow.json` with its runner and build behavior. Use `mode: "manifest"` for metadata-only plugins. Packaged plugins declare a structured build `command` argv; plugin-specific preparation belongs in a script under that plugin directory rather than in the shared workflow.

An optional `smoke` definition can verify either a payload file or execute a payload command after the generic install + verify sequence. The publish job derives immutable artifact names directly from the generated signed manifest instead of maintaining plugin-specific filename globs.

Run the same definition checks locally with:

```bash
node scripts/plugin-workflow.mjs validate
node --test scripts/plugin-workflow.test.mjs
```

The validation requires workflow definitions, source manifests, and the official registry index to stay in sync.

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

A wrapper capability requires `process/execute`. Exactly one enabled wrapper may apply to a command; conflicting providers fail instead of being resolved by install order.

Packaged wrappers use the schema-1 subprocess protocol with three operations:

```text
can_wrap
rewrite
security_projection
```

For each operation the core sends a bounded JSON request such as:

```json
{
  "schema": 1,
  "operation": "can_wrap",
  "tool": "run_command",
  "command": "git push --force origin main"
}
```

`can_wrap` returns `{ "schema": 1, "can_wrap": true|false }`. `rewrite` and `security_projection` return `{ "schema": 1, "command": "..." }`. Packaged wrapper entrypoint basenames must match the capability suffix, rewrites must be transparent `<wrapper> <requested command>` projections, and `security_projection` must return the exact original requested command.

The official `rtk` plugin is host-backed instead. RTK-specific checks and behavior live entirely in `plugins/rtk/plugin.json`: the manifest checks `rtk --version`, `rtk gain`, and a rewrite probe, declares which rewrite/passthrough exit codes are valid, and provides platform-specific installation recommendations. Core has no RTK-specific protocol or version logic.

At runtime the generic host-wrapper harness expands the manifest-declared `command_wrapper.args` and executes the declared host executable. For RTK that metadata maps the request through `rtk rewrite <requested command>`. Commands declared as passthrough remain unchanged; rewritten output must still execute through the declared wrapper executable. ChatGPT MCP always keeps the exact original request as the security projection:

```text
git push --force origin main
→ rtk git push --force origin main
security: git push --force origin main
```

Core guard, containment, and approval policy therefore remain authoritative. RTK rewrite exit status does not grant or bypass an approval. If the host RTK executable later disappears or fails a manifest-declared prerequisite check, plugin reconciliation disables the RTK plugin.

Both packaged and host-backed wrapper execution use bounded output, a timeout, and the reduced plugin environment. Wrapper subprocesses never inherit approval capability.

## Security invariants

Plugin capabilities are subordinate to core policy:

- plugins cannot disable the control guard or manufacture approvals;
- plugin rewrites cannot weaken command classification;
- plugins cannot expand registered workspace roots by returning different paths;
- plugin installation/trust configuration is a local operator action, not an Agent self-grant path;
- workspace scope does not weaken signature, publisher, permission, or extraction checks;
- workspace plugin payloads cannot escape `.cgm/plugins`;
- signed metadata is verified before activation; packaged artifacts are SHA-256 verified before extraction and installed payload trees are re-verified by `plugin verify`, enable, rollback, and reconciliation;
- activation records bind registry, publisher, manifest digest, platform integrity digest, version, and enabled state, while each installed version retains verified trust metadata for offline rollback;
- corrupt or unverifiable activation state is disabled instead of being guessed/reconstructed from executable files;
- plugin subprocesses receive a reduced environment and do not inherit control-plane approval authority;
- trace, logger, and persisted runtime-event paths apply shared secret redaction before diagnostic data is emitted or stored.

These rules protect the `chatgpt-mcp` application boundary. They do not make arbitrary same-user native plugin code harmless; use OS isolation for that threat model.
