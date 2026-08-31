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
```

The shell policy also recognizes common nested-shell/wrapper patterns rather than checking only a direct `cgm` command.

## Protected config/state subtree

The selected config root contains control-plane material such as runtime control credentials, tunnel secrets, OAuth state, and configuration.

Built-in MCP shell/file paths deny direct access to the protected control-plane subtree, including canonicalized path aliases/symlinks.

This prevents an Agent from simply reading the local runtime-control token and using it to bypass CLI policy.

## What this boundary does not provide

A process deliberately running arbitrary native code as the same OS user may have capabilities beyond what an application-level tool policy can reliably contain.

If you need a strong boundary against hostile local code, use an OS-level sandbox, container/VM boundary, or a separate operating-system identity with only the required filesystem access.

## MCP and Admin authentication

`chatgpt-mcp` stores token hashes, not plaintext MCP/admin tokens.

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

`0.0.0.0` is intentionally stricter: wildcard exposure is rejected unless both MCP and Admin authentication are enabled with configured tokens.

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

Tunnel configuration stores the runtime key separately from the main config and normal inspection output redacts sensitive values.

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

## Managed services and privilege

On Linux/macOS:

```bash
cgm up --system
```

uses system-level service registration. The CLI elevates only the service-management operation through `sudo` when needed, while the MCP process itself is configured to run as the invoking user rather than root.

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
