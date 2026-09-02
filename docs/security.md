# Security

`chatgpt-mcp` is designed to expose useful local capabilities while keeping control-plane changes and filesystem scope explicit.

This document describes the built-in security boundaries. It is not a substitute for OS-level sandboxing when running deliberately hostile code under the same operating-system user.

## Workspace boundary

Filesystem, shell mutation, Git/process working directories, and checkpoint/rewind validation use one canonical effective root set:

```text
registered workspace root
+ global permissions.allow_dirs
+ workspace-specific allow_dirs
```

Register a workspace:

```bash
cgm workspace register ~/projects/my-project
```

Grant a workspace one additional root:

```bash
cgm workspace access add ws_... /path/to/build-cache
```

Grant all workspaces a global root:

```bash
cgm config set permissions.allow_dirs /tmp,/var/tmp/chatgpt-mcp
```

Paths are canonicalized and symlink escapes are rejected.

Revoking an allowed root also prevents old checkpoints from restoring files back into that revoked path.

## MCP tool self-grant prevention

Shell/process descendants launched by MCP tools are marked as MCP tool execution context.

In that context, the `cgm` CLI is fail-closed. Read-only inspection is allowed, while control-plane mutations are denied.

Examples of read-only operations:

```text
status
config get/list
config path
config verify
workspace list/show/access list
auth status
mcp inspection
tunnel status
cluster status
logs
logs follow
logs path
version
```

Examples of denied control-plane mutations:

```text
up
down
_service
logs clear
config set
config reload
init
uninit
auth changes
workspace register/unregister/access grants
upstream MCP mutations
tunnel configuration/enable/disable
cluster relay startup/configuration changes
```

The shell policy also recognizes common nested-shell/wrapper patterns rather than checking only a direct `cgm` command. It rejects direct attempts to clear the MCP tool-context marker. On Linux, the CLI additionally inspects the process ancestry for the marker, so a child script cannot regain control-plane mutation access merely by deleting the variable from the environment passed to `cgm`.

## Protected config/state subtree

The selected config root contains control-plane material such as ephemeral runtime-control state, keyring references/metadata, OAuth state metadata, and configuration. Long-lived reversible credentials themselves are stored in the OS keyring.

Built-in MCP shell/file paths deny direct access to the protected control-plane subtree, including canonicalized path aliases/symlinks.

This prevents an Agent from simply reading the local runtime-control token and using it to bypass CLI policy.

## What this boundary does not provide

A process deliberately running arbitrary native code as the same OS user may have capabilities beyond what an application-level tool policy can reliably contain. Process-ancestry checks are defense in depth, not a sandbox: hostile code may deliberately daemonize, create a new session, manipulate process state, or access files directly without invoking `cgm`.

If you need a strong boundary against hostile local code, use an OS-level sandbox, container/VM boundary, or a separate operating-system identity with only the required filesystem access.

## MCP and Admin authentication

`chatgpt-mcp` stores MCP/Admin app token hashes, not their plaintext bearer tokens. Long-lived reversible credentials use the OS keyring: Windows Credential Manager, macOS Keychain, and on Linux Secret Service (`org.freedesktop.secrets`) with the kernel user keyring as a headless fallback.

On Linux, Secret Service is preferred because it persists credentials across reboot. When no Secret Service provider is available, headless systems fall back to the kernel user keyring without requiring a desktop session or D-Bus. Kernel keyring entries are memory-backed and do not survive reboot; installing a Secret Service provider is recommended for reboot-persistent credentials. Neither backend falls back to plaintext files.

Create/rotate:

```bash
cgm auth mcp create
cgm auth admin create
```

Inspect without exposing hashes:

```bash
cgm auth status
```

Direct clients authenticate enabled endpoints with:

```http
Authorization: Bearer <token>
```

MCP and Admin authentication are separate policies.

## Network exposure

Default exposure is loopback-only.

Persist exposure:

```bash
cgm config set server.expose none
cgm config set server.expose all
cgm config set server.expose eth0,tailscale0
cgm config set server.expose 0.0.0.0
```

Any exposure beyond loopback (`all`, an explicit interface list, or `0.0.0.0`) is rejected unless MCP authentication is enabled with a configured token. If the Admin endpoint is enabled, Admin authentication with a configured token is required as well.

`all` and explicit interface exposure include eligible IPv4 and IPv6 global-unicast addresses discovered on those interfaces. The explicit `0.0.0.0` mode remains IPv4 wildcard exposure and only advertises IPv4 endpoints.

Direct listeners currently use HTTP rather than built-in TLS. Non-loopback exposure therefore also requires an explicit `server.allow_insecure_http=true` acknowledgement. Bearer credentials and request contents are not transport-encrypted by `chatgpt-mcp` itself; use this only on a trusted or already encrypted network (for example, an appropriate private overlay), or terminate TLS in a reverse proxy. Prefer Secure MCP Tunnel when public ingress is unnecessary.

Prefer Secure MCP Tunnel for ChatGPT connectivity when the MCP runtime should remain private instead of opening the MCP listener to the public internet.

## OpenAI Secure MCP Tunnel credentials

Keep these concepts separate:

```text
tunnel_id          non-secret identifier
runtime API key    secret used by the embedded tunnel client
Admin API key      secret used for Platform admin/tunnel CRUD workflows
MCP token          secret used by direct MCP HTTP clients
Admin token        secret used by the local admin endpoint
```

For the normal Secure MCP Tunnel runtime, use a restricted OpenAI runtime API key with:

```text
Tunnels Read + Use
```

Do not use an OpenAI Admin API key as the long-lived runtime key.

Tunnel runtime/admin keys are stored in the OS keyring. `tunnel.<ext>` contains only configured-state markers and admin scope metadata, and normal inspection output redacts sensitive values. Legacy plaintext credentials can be migrated explicitly with `cgm config migrate`; normal credential-loading paths also migrate legacy values before rewriting their files.

See [OpenAI + ChatGPT setup](openai-chatgpt.md).

## Cluster relay security

Cluster relay connections use one shared bearer token. Runtime config stores that token through secret storage; Admin responses never return it. For dedicated service/container deployments, `cgm cluster relay --token-file <path>` reads the token from a mounted credential file and avoids putting the secret in process arguments or plaintext config.

The relay does not provide built-in TLS. `cgm cluster relay` refuses a non-loopback listener unless `--allow-insecure-http` is explicitly supplied. For remote runtimes, put the relay behind TLS and configure `wss://.../cluster`.

Relay endpoints:

- `/cluster` — authenticated WebSocket protocol
- `/health` — unauthenticated liveness/readiness JSON; keep returned data non-secret
- `/metrics` — authenticated operational JSON using the same relay bearer token

The relay enforces a connection cap, per-connection request rate limit, 4 MiB WebSocket read limit, initial-hello timeout, idle timeout, and write timeout. Defaults can be overridden with `cgm cluster relay --help`.

The shipped relay backend is in-memory and supports one authoritative relay process. Do not run multiple independent relays for the same tunnel-leader cluster and treat them as active-active: their leadership state is independent. Runtime reconnect plus a service supervisor is the supported availability model until a distributed relay backend is provided.

See [Cluster federation](cluster.md).

## Tunnel network model

OpenAI Secure MCP Tunnel uses outbound HTTPS from the machine running `chatgpt-mcp` to OpenAI's control plane.

For the normal private setup you do not need inbound public reachability to the local MCP server.

The host still needs:

- outbound HTTPS to the OpenAI tunnel control plane
- local/private reachability to the tools and workspaces the MCP runtime uses

## Runtime control channel

A running runtime exposes an authenticated control endpoint on loopback only. It is used internally for operations such as:

- reload
- status
- shutdown
- live events
- safe journal clearing

The random bearer credential is stored under the protected selected config root.

## Runtime journal sanitization

Persistent runtime events are sanitized before writing.

The journal is not intended to retain:

- Authorization headers
- MCP/admin tokens
- OpenAI tunnel API keys
- token hashes
- raw file contents
- full arbitrary tool argument payloads containing secrets

Structured metadata such as component, event name, workspace, tool, status, source, and duration can be retained for operations/debugging.

Use:

```bash
cgm logs path
```

to locate the selected instance's journal.

## Config/state isolation

For CI, tests, and development commands that mutate state, always use an isolated config root:

```bash
CHATGPT_MCP_CONFIG_DIR="$(mktemp -d)" go test ./...
```

or:

```bash
cgm --config-dir /tmp/cgm-test init
```

The repository test/smoke workflow treats avoiding the real default config directory as an invariant.

## Admin browser session

The embedded Admin UI keeps the Admin bearer token in browser `sessionStorage`, not persistent `localStorage`, so closing the browser session clears the stored credential. The Admin handler also sends a restrictive Content Security Policy plus frame, MIME-sniffing, referrer, and browser-permission hardening headers. This reduces browser-side token exposure but does not replace HTTPS when the Admin listener is reachable over a network.

## Upstream OAuth network policy

The origin of an upstream MCP URL explicitly configured by the user is trusted for that upstream, including local/private development servers. OAuth resource metadata, issuer, token, registration, and redirect targets advertised outside that configured origin must use HTTPS and must not resolve to loopback, private, link-local, unspecified, or multicast addresses. Redirects are checked with the same policy to prevent a public OAuth endpoint from pivoting requests into local infrastructure or cloud metadata services.

## Managed services and privilege

On Linux/macOS:

```bash
cgm up --system
```

uses system-level service registration. The CLI elevates only the service-management operation through `sudo` when needed, while the MCP process itself is configured to run as the invoking user rather than root. Linux managed services also use systemd `NoNewPrivileges=true`, so the runtime and commands launched by it cannot gain additional privilege through setuid/setgid binaries or file capabilities. Commands that genuinely require privilege escalation should be run outside the managed MCP runtime by the operator.

On Windows, `cgm up` uses a per-user Scheduled Task with least privilege and does not run as LocalSystem.

See [Runtime and services](runtime.md).

## Recommended operational defaults

- Keep `server.expose` at `none` unless direct network clients genuinely need access.
- Prefer OpenAI Secure MCP Tunnel instead of public MCP ingress for ChatGPT.
- Use restricted tunnel runtime keys with Read + Use only.
- Register only the workspace roots ChatGPT needs.
- Add extra directories narrowly and per-workspace where possible.
- Keep Admin auth enabled whenever the Admin endpoint is reachable beyond loopback.
- Review `cgm logs --debug` when diagnosing access or tunnel behavior, but avoid publishing raw diagnostic logs without checking their contents.
- Run risky tool workloads in an OS sandbox/separate identity when application-level workspace controls are not a sufficient trust boundary.
