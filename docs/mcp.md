# MCP and upstreams

`chatgpt-mcp` implements the stateless MCP `2026-07-28` protocol revision and can also aggregate tools from configured upstream MCP servers.

## Local MCP endpoint

Default endpoint:

```text
http://127.0.0.1:37421/mcp
```

Modern requests use `POST /mcp` and carry protocol/routing metadata on each request.

Typical headers:

```http
MCP-Protocol-Version: 2026-07-28
Mcp-Method: tools/list
Content-Type: application/json
```

Typical request metadata:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list",
  "params": {
    "_meta": {
      "io.modelcontextprotocol/protocolVersion": "2026-07-28",
      "io.modelcontextprotocol/clientCapabilities": {},
      "io.modelcontextprotocol/clientInfo": {
        "name": "example-client",
        "version": "1.0.0"
      }
    }
  }
}
```

## Protocol behavior

The runtime supports the current project surface including:

- `server/discover`
- `tools/list`
- `tools/call`
- `subscriptions/listen`
- per-request client/protocol metadata
- SEP-2243 `Mcp-Method`, `Mcp-Name`, and `Mcp-Param-*` validation
- Multi Round-Trip Requests (MRTR)

The protocol revision is stateless:

- no `initialize` handshake
- the direct stateless endpoint does not create `Mcp-Session-Id`; when a client or Secure MCP Tunnel supplies one, the runtime uses it to track the session's ephemeral multi-workspace access set and approval identity
- unknown/removed methods return HTTP `404` with JSON-RPC method-not-found semantics
- `GET` and `DELETE` on the MCP endpoint return `405`

## Local authentication

When MCP authentication is enabled, direct HTTP clients use:

```http
Authorization: Bearer <mcp-token>
```

Create or rotate the token with:

```bash
cgm auth mcp create
```

The OpenAI Secure MCP Tunnel runtime key is unrelated to this local MCP bearer token. The tunnel key authenticates the embedded tunnel client to OpenAI's control plane.

## Workspace-bound tools

Tools that operate on the filesystem, shell, Git, processes, rules, skills, context, or checkpoints require an explicit registered workspace handle where applicable.

Register:

```bash
cgm workspace register ~/projects/my-project
```

The workspace ID is stable by canonical path and does not silently switch to another project. Older instance-scoped IDs from registry v2 are migrated and retained as aliases.

For requests carrying an MCP session ID, each valid explicitly targeted workspace is added to that session's in-memory access set. The same session can therefore work across multiple registered projects without a workspace-switch operation. Every scoped call still requires `workspace_id`, and workspace-specific context, filesystem scope, shell/REPL state, checkpoints, and approvals remain isolated by that target.

Effective filesystem scope and session isolation are described in [Security](security.md).

## Effective project context and global instructions

`project_context` is assembled by one shared builder used by both the MCP tool and the Admin workspace preview. Its effective instruction text can include project-local instruction files, managed global context/rules, enabled user-level instruction sources, matching path rules, optional skill metadata, Git context, and memory according to the tool options and configured byte/line budgets.

The Admin `Global Instructions` view manages the runtime-owned global context and always-on rules. It also discovers supported user-level instruction providers on the current machine and exposes policy switches only for resource kinds that actually exist. A missing provider or missing context/rules/skills kind is not synthesized into the UI. Disabling a detected user-level source is enforced in the shared discovery/load path used by `project_context`, `list_skills`, `load_skill`, and `load_path_rules`; project-local sources remain independent from those user-level switches.

The Admin workspace `Context` view calls the same builder as the MCP tool, so its rendered preview is intended to represent the effective context a corresponding `project_context` call would receive with the same options.

## Synchronous commands and Admin execution streaming

`run_command` remains a synchronous MCP tool: the caller receives the normal final command result with stdout, stderr, cwd, exit code, and timeout state. While the command is running, the runtime additionally mirrors stdout/stderr into a workspace-scoped in-memory execution stream for the Admin workspace `Activity` view.

Admin execution streams use a bounded output tail and bounded recent-execution history. Reconnecting clients first receive a full execution snapshot and then sequence-numbered output/completion events, so they can recover from a dropped or overflowed SSE connection without changing the MCP result contract. Raw streamed stdout/stderr is intentionally excluded from the normal activity observation payload; the activity journal keeps command/result metadata while live command output stays in the execution buffer.

## Control-guard approval flow

When a workspace-scoped tool attempts an approvable control-plane mutation, the tool call returns structured `approval_required` content instead of executing the mutation. The response includes a short-lived `challenge_id`, the workspace, target tool, exact canonical arguments, guard reason, and the `request_control_approval` tool name.

The agent may then call:

```text
request_control_approval(workspace_id, challenge_id)
```

That request is accepted only when the challenge came from a real guard failure in the same MCP session and workspace. The human request expires after 60 seconds and can be reviewed in the Admin UI or with `cgm request list/view`. Approval or denial is performed outside the agent's MCP tool context.

If approved, the tool response instructs the agent to retry the original target tool. The retry must match the approved session, workspace, target tool, source, guard code, and arguments exactly. A mismatched retry returns `approval_mismatch` and does not consume the valid approval; an exact retry consumes it once. Hard-deny guards such as protected-state access, path escape, nested/wrapper control-plane commands, session/workspace rebinding, and tool-context tampering never produce an approval challenge.

For direct `cgm` execution, the approved retry is additionally narrowed to one opaque, short-lived child capability bound to the exact CLI argv. The MCP tool-context marker remains present; the child CLI verifies the capability over authenticated loopback runtime-control before executing.

See [Security](security.md#control-guard-approvals-and-self-grant-prevention) for the full trust model.

## Upstream MCP aggregation

`chatgpt-mcp` can connect to other MCP servers and expose enabled upstream tools through the same local tool catalog.

Start with:

```bash
cgm mcp --help
cgm mcp server --help
```

Upstream definitions can also be managed in the embedded admin dashboard.

Common management commands:

```bash
cgm mcp server list
cgm mcp server show <id>
cgm mcp server status <id>
cgm mcp server tools <id>
cgm mcp server enable <id>
cgm mcp server disable <id>
cgm mcp server remove <id>
```

Add an HTTP upstream:

```bash
cgm mcp server add example \
  --transport http \
  --url https://mcp.example.com/mcp \
  --auth auto \
  --expose all
```

Add a local stdio upstream:

```bash
cgm mcp server add local-tools \
  --transport stdio \
  --command node \
  --arg /path/to/server.mjs \
  --cwd /path/to/project \
  --expose all
```

Useful add/configure controls include:

- `--tool-prefix <prefix>` for proxy tool names
- `--tool <name>` to allowlist tools
- `--disable-tool <name>` to hide tools
- `--expose none|meta_only|allowlist|all`
- `--bearer-token-env <ENV_NAME>` for HTTP bearer auth without storing the token in config
- `--header KEY=VALUE` for HTTP headers
- `--env KEY=VALUE` for stdio environments
- `--idle-timeout <seconds>`

Update selected fields with:

```bash
cgm mcp server configure <id> [flags]
```

`configure` also has the alias `set`.

## Upstream OAuth

HTTP upstreams can use OAuth. Access/refresh tokens and client secrets are stored in the selected config root's secret-file store; the structured OAuth state file contains only non-secret metadata and `<secret-file>` markers.

```bash
cgm mcp server auth login <id>
cgm mcp server auth status <id>
cgm mcp server auth logout <id>
```

Upstream tool discovery and proxy refresh are designed to be atomic:

1. build the complete replacement proxy set
2. validate required upstream discovery/schema state
3. commit one registry swap only after the replacement is ready

If discovery or schema refresh fails, the previous working proxy catalog remains active instead of being partially replaced.

## Tool catalog changes

The exposed tool catalog can change when:

- upstream MCP servers are added/removed/updated
- upstream discovery succeeds with a changed schema
- built-in features are toggled
- permissions/configuration alter available tools

Subscribers can observe catalog changes through the MCP subscription surface.

## MRTR

The runtime supports Multi Round-Trip Requests with the project's current fields such as:

```text
input_required
inputRequests
inputResponses
requestState
```

Upstream MRTR behavior can be relayed through the proxy path where supported.

## OpenAI Secure MCP Tunnel transport

For private ChatGPT connectivity, the embedded OpenAI tunnel client connects the local MCP runtime to an OpenAI-hosted tunnel endpoint without requiring the local `/mcp` listener to be public.

The high-level path is:

```text
ChatGPT
  -> OpenAI-hosted tunnel endpoint
  -> embedded tunnel client
  -> chatgpt-mcp MCP runtime
  -> tools / workspaces / upstream MCPs
```

See [OpenAI + ChatGPT setup](openai-chatgpt.md).

## Version inspection

The MCP tool catalog includes a read-only runtime version tool where supported by the current binary. The CLI also exposes:

```bash
cgm version
cgm --version
```

Version metadata can include build version, commit, and build time depending on how the binary was produced.
