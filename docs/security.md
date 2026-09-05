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

## MCP session workspace isolation

One MCP session may access multiple registered workspaces. Every workspace-scoped call must explicitly carry a valid `workspace_id`; the runtime does not infer a current workspace or silently switch arguments. A valid target is added to the session's ephemeral in-memory workspace access set, while an invalid workspace is rejected before tool execution.

Multi-workspace access does not merge workspace state. Filesystem roots, shell cwd, Git/process working directories, project context, rules, memory, Node REPL state, checkpoints, and approvals remain scoped by the explicitly targeted workspace. Path containment and symlink checks still apply independently on every call. Session access sets are refreshed while active, expire after 30 days of inactivity, and are not persisted to disk.

Activity/log observability stores only a short SHA-256-derived session fingerprint, whether the targeted workspace access was `new` or `existing`, and the number of workspaces seen by that session; raw MCP session IDs are not exposed.

## Control-guard approvals and self-grant prevention

Shell/process descendants launched by MCP tools are marked as MCP tool execution context. Read-only inspection remains allowed, while control-plane mutations are guarded.

A narrow subset of direct literal CLI mutations can be elevated by a human without weakening the surrounding sandbox. The flow is challenge-driven:

```text
MCP tool call -> typed control guard -> approval_required + challenge_id
             -> request_control_approval(challenge_id)
             -> local human approve/deny
             -> exact retry of the original tool call
             -> one-shot child capability -> exact cgm argv
```

The initial guard failure does not create a human request by itself. The agent must call `request_control_approval` with the challenge returned by that exact guarded call. Challenges are short-lived (30 seconds), pending human requests expire after 60 seconds, and an approved retry window lasts 30 seconds. The approval is bound to the runtime instance, MCP session, workspace, source, target tool, canonical arguments, and guard code.

An approved retry must match the original tool call exactly. A mismatch returns an `approval_mismatch` response containing expected/actual target information and leaves the valid grant unconsumed so the agent can retry the approved payload. A successful retry consumes the grant atomically. For direct `cgm` execution, the runtime then mints a separate opaque child capability valid for 15 seconds and one exact argv; `CHATGPT_MCP_TOOL_CONTEXT=1` remains set. The child CLI verifies and consumes that capability over authenticated loopback runtime-control before allowing the mutation.

Human approval never grants a general shell or CLI bypass. Only typed direct control-plane mutations marked approvable by the guard can enter this flow. These remain hard-denied and cannot create an approval request:

- path escape or protected config/state access
- attempts to clear or tamper with the MCP tool-context marker
- nested shell/wrapper/compound commands where the exact child mutation cannot be safely bound
- attempts to bypass explicit workspace/session scoping
- request approval/deny commands invoked from MCP tool context
- other guards explicitly marked non-approvable

The approved child process executes the current server binary directly instead of resolving `cgm` through workspace `PATH`, preventing a workspace-supplied executable from receiving the capability.

Local operators can review requests with `cgm request list/view`, approve or deny them with `cgm request approve/deny`, or use the Admin UI dialog. The agent cannot approve its own request through the built-in shell because those resolution commands are themselves hard-denied in MCP tool context.

## Protected config/state subtree

The selected config root contains control-plane material such as ephemeral runtime-control state, OAuth/upstream state, secret files, and configuration. Long-lived reversible credentials are stored under `<config-root>/state/secrets/` as one file per secret, keyed by a SHA-256-derived filename and written atomically with restrictive file/directory modes where supported.

Built-in MCP shell/file paths deny direct access to the protected control-plane subtree, including canonicalized path aliases/symlinks.

This prevents an Agent from simply reading the local runtime-control token and using it to bypass CLI policy.

## What this boundary does not provide

A process deliberately running arbitrary native code as the same OS user may have capabilities beyond what an application-level tool policy can reliably contain. Process-ancestry checks are defense in depth, not a sandbox: hostile code may deliberately daemonize, create a new session, manipulate process state, or access files directly without invoking `cgm`.

If you need a strong boundary against hostile local code, use an OS-level sandbox, container/VM boundary, or a separate operating-system identity with only the required filesystem access.

## MCP and Admin authentication

`chatgpt-mcp` stores MCP/Admin app token hashes, not their plaintext bearer tokens. Long-lived reversible credentials such as tunnel keys, OAuth tokens/client secrets, and sensitive upstream header/environment values are stored in the selected config root's `state/secrets/` directory instead of structured config files. Structured files keep `<secret-file>` markers and non-secret metadata.

The secret store is intentionally file-backed and has no OS keyring dependency. Keep the selected config root private to the operating-system user. Legacy `<os-keyring>` markers are recognized as legacy state so configuration can fail clearly, but values that existed only in a removed OS-keyring backend cannot be recovered and must be configured again.

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

Tunnel runtime/admin keys are stored in the per-config-root secret-file store. `tunnel.<ext>` contains only configured-state markers and admin scope metadata, and normal inspection output redacts sensitive values. Legacy plaintext credentials can be migrated explicitly with `cgm config migrate`; normal credential-loading paths also migrate legacy values before rewriting their files.

See [OpenAI + ChatGPT setup](openai-chatgpt.md).

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
- approval request list/detail/approve/deny operations
- one-shot approved child capability verification/consumption

The random bearer credential is stored under the protected selected config root. Approval challenges, requests, retry grants, and child capabilities are runtime-memory state and are not persisted; restarting the runtime invalidates them.

Approval resolution has a stricter remote Admin boundary than ordinary local browsing. Loopback Admin clients follow the local policy, but a non-loopback approval mutation requires enabled Admin authentication and a valid Admin bearer token. Forwarded-address headers are not trusted to turn a remote caller into loopback.

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
