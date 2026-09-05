# Configuration

`chatgpt-mcp` keeps persistent configuration and runtime state under one selected config root. JSON is the default format; YAML and TOML are also supported.

## Config root

Default:

```text
~/.config/chatgpt-mcp/
```

Override it with:

```bash
cgm --config-dir /path/to/instance status
```

or:

```bash
export CHATGPT_MCP_CONFIG_DIR=/path/to/instance
cgm status
```

Precedence:

```text
--config-dir
    >
CHATGPT_MCP_CONFIG_DIR
    >
default user config root
```

The selected root covers configuration, non-secret credential metadata, upstream/OAuth state, workspace registry, shell state, memory, checkpoints, logs, runtime control state, and the per-root secret-file store under `state/secrets/`. Long-lived reversible credentials are namespaced to this config root and stored there instead of structured config.

Use a non-default root for tests, temporary instances, and development binaries that mutate configuration.

## Initialize and choose format

```bash
cgm init
cgm init --json
cgm init --yaml
cgm init --toml
cgm init --format toml
```

The main configuration determines the serialization format used by managed structured state files.

## Inspect configuration

```bash
cgm config get
cgm config list
cgm config get admin.enabled
cgm config list admin
```

Structured output can be selected independently from the on-disk format:

```bash
cgm config list --json
cgm config list --yaml
cgm config list --toml
cgm config list --format yaml
cgm config get admin --toml
```

Sensitive values keep their real key names but render as:

```text
<redacted>
```

## Set values

```bash
cgm config set server.port 41021
cgm config set admin.port 41022
cgm config set admin.enabled true
```

`key=value` syntax is also accepted by the CLI.

Changes are validated before persistence.

## Presets

Built-in presets provide named baseline configurations while preserving configured secrets and tunnel details when applied.

```bash
cgm config preset list
cgm config preset show <name>
cgm config preset current
cgm config preset apply <name>
```

Use `current` to see whether the active configuration still matches a known preset or has become `custom`.

## Reload a running runtime

After persisting a change:

```bash
cgm config reload
```

The reload path uses the selected config root's loopback-only runtime control channel.

Changes to auth, feature flags, filesystem permissions, and tunnel settings can be applied live.

Changes to these network settings trigger listener rebind inside the same process:

- `server.port`
- `server.expose`
- `admin.enabled`
- `admin.port`

Rebind is transactional. If a requested port/address cannot be opened, the previous listener set is restored.

## Migrate legacy credentials

```bash
cgm config migrate
```

This moves legacy plaintext tunnel keys, OAuth credentials, and sensitive upstream header/environment values into the per-config-root secret-file store and rewrites structured state with non-secret `<secret-file>` markers. Normal credential-loading paths also migrate automatically. The secret store has no OS-keyring dependency; migration fails rather than retaining a reversible secret in structured config when the secret file cannot be written safely.

## Verify config/state

```bash
cgm config verify
cgm config validate
```

`verify` and `validate` are aliases.

Verification checks:

- main config exists
- managed structured files use the expected format/extension
- files decode successfully
- loaded runtime configuration passes semantic validation

## Convert formats

```bash
cgm config convert json
cgm config convert yaml
cgm config convert toml
cgm config transform toml
```

`convert` and `transform` are aliases.

The operation preflights the managed structured state tree before mutation and rolls back if persistence fails.

## Portable export and import

Export the selected config root into one portable bundle:

```bash
cgm config export
```

Import it on another supported machine:

```bash
cgm config import
```

When no file is supplied, both commands use `chatgpt-mcp-config.cgm` in the current directory. A custom path remains supported:

```bash
cgm config export laptop.cgm
cgm config import laptop.cgm
```

The `.cgm` bundle is platform-neutral and can move in any direction between supported Linux, macOS, and Windows installations, including between amd64 and arm64 machines. It contains the persistent configuration/state that can meaningfully be restored plus all currently managed reversible secrets. MCP/Admin token hashes are preserved as part of the config, so existing endpoint tokens keep working even though their plaintext values are not stored by `chatgpt-mcp`.

Secrets are serialized by logical secret name and recreated through the destination secret store. Raw `state/secrets/*` files are never copied, because their on-disk names are namespaced to the source config root and are not portable across machines.

The bundle is compressed and sealed with authenticated encryption using an application-level key and a random nonce. It requires no password and rejects modified/corrupted ciphertext, but it is intentionally a portability/obfuscation boundary rather than password-grade secret storage: possession of both the bundle and a compatible `chatgpt-mcp` binary should not be treated as strong cryptographic separation.

Filesystem state is normalized for the destination machine:

- paths under the source user's home are mapped to the same relative location under the destination user's home when that directory exists
- other absolute paths are kept only on the same OS when they still exist
- unavailable `permissions.allow_dirs`, `shell.path`, workspace roots, and workspace allow directories are skipped
- mapped workspaces receive the stable ID for their destination path, retain the source ID as a legacy alias, and portable workspace state such as auto memory follows the new ID
- interface-specific network exposure is reset to loopback-only when moving between different OSes

The bundle intentionally excludes transient or machine-owned state that should be regenerated on the destination: runtime control state/PIDs, runtime logs, managed-service environment snapshots, instance identity, shell session state/history, checkpoints, update cache, and service-manager definitions. The original source values remain inside the bundle only where they are part of portable data; import never requires the source filesystem to exist.

Export refuses to overwrite an existing output file unless requested explicitly:

```bash
cgm config export --force
```

Import refuses to replace an existing config root unless requested explicitly:

```bash
cgm config import --force
```

Import must run while the selected runtime is stopped. It stages the imported tree, swaps it into place, restores logical secrets, verifies the resulting configuration, and restores the previous config root if activation or verification fails.

## Network exposure

Default exposure is loopback-only:

```json
{
  "server": {
    "port": 37421,
    "expose": {
      "mode": "none",
      "interfaces": []
    }
  }
}
```

Supported modes:

| Mode | Behavior |
| --- | --- |
| `none` | bind loopback only |
| `all` | loopback plus currently active eligible IPv4 interfaces |
| `interfaces` / interface names | loopback plus selected active interfaces |
| `0.0.0.0` | wildcard IPv4 listener including interfaces that appear later |

One-run override:

```bash
cgm serve --expose
cgm serve --expose=all
cgm serve --expose=0.0.0.0
cgm serve --expose=eth0
cgm serve --expose=eth0,tailscale0
cgm serve --expose=none
```

Bare `--expose` means `all`.

Persist the policy:

```bash
cgm config set server.expose all
cgm config set server.expose 0.0.0.0
cgm config set server.expose eth0,tailscale0
cgm config set server.expose none
```

Selected interfaces must be active, non-loopback, and expose an eligible IPv4 address. Startup fails rather than silently broadening exposure when a configured interface is unavailable.

Wildcard `0.0.0.0` is rejected unless both MCP and Admin authentication are enabled with configured tokens.

## Authentication

`chatgpt-mcp` stores token hashes rather than plaintext MCP/admin tokens. Plain tokens are shown only when created or rotated.

```bash
cgm auth mcp create
cgm auth admin create
cgm auth status
cgm auth mcp enable
cgm auth mcp disable
cgm auth admin enable
cgm auth admin disable
```

Use an enabled endpoint token as:

```http
Authorization: Bearer <token>
```

MCP and Admin authentication are independent policies except for wildcard exposure, which requires both.

The Admin UI skips its login screen when Admin authentication is disabled.

## Workspace filesystem scope

Register a workspace:

```bash
cgm workspace register ~/projects/my-project
```

Workspace IDs are stable hashes of canonical workspace paths. Older registry-v2 instance-scoped IDs are migrated to the stable ID and retained as aliases. The runtime never guesses or falls back to another registered workspace when an ID is invalid.

An MCP session may access multiple registered workspaces. Every workspace-scoped tool call must explicitly provide a valid `workspace_id`; the runtime canonicalizes that ID and records the workspace in the session's in-memory access set. Invalid workspace IDs do not create access entries. Workspace-specific filesystem scope and state remain isolated even when the same session moves between projects.

Global extra roots apply to every workspace:

```bash
cgm config set permissions.allow_dirs /tmp,/var/tmp/chatgpt-mcp
cgm config get permissions.allow_dirs
```

Workspace-specific extra roots:

```bash
cgm workspace access add ws_... /path/to/build-cache
cgm workspace access list ws_...
cgm workspace access remove ws_... /path/to/build-cache
```

Effective filesystem scope is:

```text
workspace root
+ global permissions.allow_dirs
+ workspace-specific allow_dirs
```

Filesystem operations, shell mutation validation, Git/process working directories, and rewind/checkpoint validation use the same canonical root set. Symlink escapes remain denied.

## Feature flags

Built-in features live under `features` and can be updated through config or Admin Settings.

Examples:

```bash
cgm config set features.ponytail.enabled true
cgm config set features.caveman.enabled false
cgm config reload
```

Admin Settings applies persisted feature changes to the live tool catalog directly when possible.

## Tunnel configuration

Configure OpenAI Secure MCP Tunnel:

```bash
cgm tunnel configure \
  --enabled \
  --id tunnel_... \
  --api-key 'sk-...'
```

Optional:

```bash
cgm tunnel configure \
  --control-plane-base-url https://api.openai.com \
  --organization-id org_...
```

Tunnel keys are persisted in the per-config-root secret-file store. The companion tunnel file uses the selected serialization format only for configured-state markers and admin scope metadata, for example:

```text
config.toml
tunnel.toml
```

Tunnel configuration updates are transactional with rollback on persistence/apply failure.

For OpenAI Platform/ChatGPT setup, see [OpenAI + ChatGPT setup](openai-chatgpt.md).

## Upstream MCP configuration

Manage upstream servers with:

```bash
cgm mcp --help
cgm mcp server --help
```

Upstream OAuth access/refresh tokens and client secrets are stored in the per-config-root secret-file store. Sensitive upstream header/environment values are also moved there, while non-secret upstream configuration remains in the structured state file. Proxy refresh is atomic: the old exposed proxy catalog remains active if replacement discovery/schema construction fails.

See [MCP and upstreams](mcp.md).

## Remove config/state

```bash
cgm uninit
```

This removes the selected `chatgpt-mcp` config/state root. It is intentionally different from uninstalling the binary.

If a managed service is active, use `cgm down` or `cgm down --system` for the matching Linux/macOS system scope before removing the instance.
