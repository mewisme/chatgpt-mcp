# Configuration Fields

The Config TUI is schema-driven. Each field below is the same key exposed by the application's configuration field registry. Editable fields use the field type declared by the schema; managed credential/scope fields are read-only.

## Runtime & Network

### `server.enabled` — MCP HTTP server

Boolean. Controls whether the local MCP HTTP transport is enabled. Disabling it removes local HTTP MCP connectivity. At least one MCP transport must remain enabled, so the Secure MCP Tunnel must be enabled before this can be disabled by itself. Related: `server.port`, `server.expose.mode`, `auth.mcp_enabled`, `tunnel.enabled`.

### `server.expose.mode` — Exposure

Enum controlling which local network addresses expose HTTP servers.

- `none`: loopback only.
- `all`: loopback plus every eligible address discovered on all interfaces.
- `0.0.0.0`: one IPv4 wildcard listener exposing eligible IPv4 addresses.
- `interfaces`: loopback plus addresses from `server.expose.interfaces`.

Non-loopback exposure also requires `server.allow_insecure_http=true` and valid authentication for each enabled HTTP endpoint.

### `server.expose.interfaces` — Exposure interfaces

List of network interface names used when exposure mode is `interfaces`. Enter one value per line in the TUI. Each name must resolve to an available interface with at least one eligible IP address at runtime. Duplicates are normalized away. Set `server.expose.mode=interfaces` before relying on this list.

### `server.port` — MCP HTTP port

Integer TCP port for the MCP HTTP server. Valid range is `1-65535`. When both MCP and admin HTTP servers are enabled, their ports must differ.

### `server.allow_insecure_http` — Allow insecure HTTP

Boolean opt-in allowing authenticated plain HTTP endpoints beyond loopback. This does not disable authentication requirements. Prefer the Secure MCP Tunnel or a TLS reverse proxy when possible.

### `admin.enabled` — Admin server

Boolean controlling the admin HTTP server. When enabled it uses `admin.port` and the same network exposure policy. If admin authentication is enabled, a configured admin credential is required.

### `admin.port` — Admin port

Integer TCP port for the admin HTTP server. Valid range is `1-65535` while the server is enabled, and it must differ from `server.port` when both HTTP servers are enabled.

## Access & Security

### `auth.mcp_enabled` — MCP authentication

Boolean token-authentication switch for the MCP HTTP endpoint. Non-loopback HTTP exposure requires MCP authentication with a configured credential.

### `auth.admin_enabled` — Admin authentication

Boolean token-authentication switch for the admin HTTP endpoint. Non-loopback exposure with the admin endpoint enabled requires admin authentication and a configured credential.

### `auth.mcp_token_hash` — MCP credential

Read-only, sensitive managed credential hash. The raw token is never exposed through config views. Manage it through the MCP authentication/token workflow rather than Config field editing.

### `auth.admin_token_hash` — Admin credential

Read-only, sensitive managed credential hash for admin HTTP authentication. Manage it through the admin authentication/token workflow.

### `permissions.allow_dirs` — Allowed directories

List of global filesystem roots that registered workspaces may access in addition to workspace-local/per-workspace allowed directories. Paths must be absolute and are normalized.

## Shell & Execution

### `shell.path` — Executable search paths

List of trusted executable directories added to shell PATH resolution. Paths must be absolute. In inherited/filtered environments they are merged before inherited PATH; strict/minimal environments rebuild PATH from these directories plus trusted system executable paths.

### `shell.approval_policy` — Approval policy

Enum controlling when shell commands require local approval. Explicit deny rules take precedence over allow rules; independent security guards may still reject commands.

- `allow`: run without approval unless an explicit deny rule matches.
- `balanced`: require approval for guarded/risky operations and external access.
- `strict`: require approval for execution not recognized as a workspace-confined static read.
- `deny`: require approval unless an explicit allow rule matches.

### `shell.approval_allow_commands` — Allow commands

List of argv-aware command patterns that bypass ordinary approval gates. The pattern language supports `*`, `?`, character classes, and standalone `**`. For compound shell input every invocation must match an allow pattern. Deny patterns and independent security guards still take precedence. Prefer narrow patterns over broad wildcards.

### `shell.approval_deny_commands` — Deny commands

List of command patterns that always force local approval. Uses the same argv-aware glob syntax as allow rules. A matching deny rule wins over a matching allow rule.

### `shell.environment_policy` — Environment policy

Enum controlling inherited parent environment variables.

- `auto`: inherit under allow/balanced approval; minimal under strict/deny.
- `inherit`: inherit parent environment except protected internal control variables.
- `filtered`: inherit ordinary variables while removing known sensitive/credential/injection-related variables.
- `minimal`: retain a small runtime/toolchain environment and rebuild PATH from trusted executable paths.

### `shell.environment_allow` — Environment allow

List of environment variable names explicitly exposed to shell commands. Matching is case-insensitive and deduplicated. It can restore ordinary variables filtered by filtered/minimal policy, but cannot override protected internal control variables. Secrets added here become available to shell processes.

### `shell.sandbox_policy` — Sandbox policy

Enum controlling OS-level filesystem sandboxing for shell execution.

- `auto`: no filesystem sandbox for allow/balanced; strict/deny uses sandboxing when supported and may fall back when unavailable.
- `off`: no filesystem sandbox; network policy remains independent.
- `required`: require a supported OS sandbox and fail when isolation cannot be established.

Current sandboxing uses Bubblewrap on supported Linux hosts.

### `shell.network_policy` — Network policy

Enum controlling external network access from shell commands.

- `auto`: normal host networking for allow/balanced; strict/deny isolates by default and can release isolation for explicitly approved network commands.
- `inherit`: use the host network namespace without shell network isolation.
- `deny`: reject detected external access and require network isolation.

## Features

### `features.ponytail.active` — Ponytail active

Boolean controlling whether Ponytail guidance is active by default.

### `features.ponytail.mode` — Ponytail mode

Enum default intensity: `lite`, `full`, or `ultra`. Lite builds the requested solution but may point out simpler alternatives; full enforces reuse/stdlib/native-first and shortest-correct implementation; ultra applies aggressive YAGNI pressure and challenges unnecessary scope.

### `features.caveman.active` — Caveman active

Boolean controlling whether compressed Caveman response style is active by default.

### `features.caveman.mode` — Caveman mode

Enum persisted response intensity: `lite`, `full`, `ultra`, `wenyan-lite`, `wenyan-full`, `wenyan-ultra`. The wenyan variants progressively increase classical-Chinese compression. Session-only aliases such as `off` or `wenyan` are not persisted values.

## Tunnel

### `tunnel.enabled` — Tunnel

Boolean controlling the OpenAI Secure MCP Tunnel transport. Enabling requires both `tunnel.id` and a configured runtime API key. It can satisfy the requirement that at least one MCP transport remains enabled when local MCP HTTP is disabled.

### `tunnel.id` — Tunnel ID

String identifier for the Secure MCP Tunnel used by this runtime. Required while tunnel transport is enabled.

### `tunnel.api_key` — Runtime API key

Read-only sensitive managed credential. The raw key is redacted from Config. Manage it from the Tunnel page.

### `tunnel.admin_key` — Admin key

Read-only sensitive credential used for control-plane management such as listing, creating, updating, and deleting managed tunnels. It is separate from the runtime API key and is managed from the Tunnel page.

### `tunnel.admin_organization_id` — Admin organization scope

Read-only verified organization scope produced by admin-key verification.

### `tunnel.admin_workspace_id` — Admin workspace scope

Read-only verified workspace scope produced by admin-key verification.

### `tunnel.admin_tenant_id` — Admin tenant scope

Read-only verified tenant scope produced by admin-key verification.

### `tunnel.control_plane_base_url` — Control-plane URL

Optional string overriding the tunnel control-plane base URL. Empty uses the default endpoint. A custom value must be an absolute HTTP/HTTPS URL with a host.

### `tunnel.organization_id` — Organization ID

Optional OpenAI organization context associated with runtime tunnel operations. This is distinct from the verified admin-key organization scope.
