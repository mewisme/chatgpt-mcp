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

The selected root covers configuration, tunnel secrets, upstream/OAuth state, workspace registry, shell state, memory, checkpoints, logs, and runtime control state.

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

Tunnel secret material is persisted separately from the main config using the same selected serialization format, for example:

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

Upstream OAuth credentials are persisted separately from normal runtime configuration. Proxy refresh is atomic: the old exposed proxy catalog remains active if replacement discovery/schema construction fails.

See [MCP and upstreams](mcp.md).

## Remove config/state

```bash
cgm uninit
```

This removes the selected `chatgpt-mcp` config/state root. It is intentionally different from uninstalling the binary.

If a managed service is active, use `cgm down` or `cgm down --system` for the matching Linux/macOS system scope before removing the instance.
