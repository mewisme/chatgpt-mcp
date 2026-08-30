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
- no `Mcp-Session-Id`
- unknown/removed methods return HTTP `404` with JSON-RPC method-not-found semantics
- `GET` and `DELETE` on the MCP endpoint return `405`

## Local authentication

When MCP authentication is enabled, direct HTTP clients use:

```http
Authorization: Bearer <mcp-token>
```

Create or rotate the token with:

```bash
cmcp auth mcp create
```

The OpenAI Secure MCP Tunnel runtime key is unrelated to this local MCP bearer token. The tunnel key authenticates the embedded tunnel client to OpenAI's control plane.

## Workspace-bound tools

Tools that operate on the filesystem, shell, Git, processes, rules, skills, context, or checkpoints require an explicit registered workspace handle where applicable.

Register:

```bash
cmcp workspace register ~/projects/my-project
```

The workspace ID is stable and does not silently switch to another project.

Effective filesystem scope is described in [Security](security.md).

## Upstream MCP aggregation

`chatgpt-mcp` can connect to other MCP servers and expose enabled upstream tools through the same local tool catalog.

Start with:

```bash
cmcp mcp --help
cmcp mcp server --help
```

Upstream definitions can also be managed in the embedded admin dashboard.

Common management commands:

```bash
cmcp mcp server list
cmcp mcp server show <id>
cmcp mcp server status <id>
cmcp mcp server tools <id>
cmcp mcp server enable <id>
cmcp mcp server disable <id>
cmcp mcp server remove <id>
```

Add an HTTP upstream:

```bash
cmcp mcp server add example \
  --transport http \
  --url https://mcp.example.com/mcp \
  --auth auto \
  --expose all
```

Add a local stdio upstream:

```bash
cmcp mcp server add local-tools \
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
cmcp mcp server configure <id> [flags]
```

`configure` also has the alias `set`.

## Upstream OAuth

HTTP upstreams can use OAuth. Credentials/state are stored separately from normal runtime configuration.

```bash
cmcp mcp server auth login <id>
cmcp mcp server auth status <id>
cmcp mcp server auth logout <id>
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
cmcp version
cmcp --version
```

Version metadata can include build version, commit, and build time depending on how the binary was produced.
