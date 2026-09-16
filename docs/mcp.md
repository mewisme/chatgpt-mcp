# MCP clients and upstreams

For ChatGPT, the recommended/default transport is **OpenAI Secure MCP Tunnel**. Use this guide when you need a generic MCP client such as Cursor, or when `chatgpt-mcp` should aggregate tools from another MCP server.

For the ChatGPT setup, start with [OpenAI + ChatGPT](openai-chatgpt.md).

## Generic local MCP clients

`cgm mcp` starts an MCP-only transport without the normal Admin server, Secure MCP Tunnel lifecycle, or managed runtime service.

### stdio

```bash
cgm mcp stdio
```

Bind the session to one already registered workspace:

```bash
cgm mcp stdio --workspace ~/projects/my-project
cgm mcp stdio --workspace ws_...
```

Binding does not register or relocate a workspace. The target must already exist in the workspace registry.

A Cursor project configuration can therefore use:

```json
{
  "mcpServers": {
    "chatgpt-mcp": {
      "type": "stdio",
      "command": "cgm",
      "args": ["mcp", "stdio", "--workspace", "${workspaceFolder}"]
    }
  }
}
```

### Streamable HTTP

Run the dedicated loopback MCP HTTP server:

```bash
cgm mcp http
```

It exposes the current Streamable HTTP endpoint and legacy SSE compatibility. Disable SSE compatibility when unnecessary:

```bash
cgm mcp http --no-sse
```

Bind it to one registered workspace:

```bash
cgm mcp http --workspace ws_...
```

The dedicated generic-client HTTP transport is intentionally separate from the tunnel-first ChatGPT path.

## Authentication

`stdio` uses the local child-process boundary and does not require a bearer token.

Protected `cgm mcp http` uses the same Direct MCP HTTP bearer token as managed `/mcp`. Disable authentication with `cgm auth mcp disable` when a local client should connect without a token.

The OpenAI Secure MCP Tunnel runtime API key is unrelated to generic MCP client authentication.

See [Configuration](configuration.md#authentication) and [Security](security.md).

## Workspace binding

Without transport-level binding, workspace-scoped tools target explicit registered `ws_*` IDs. With `--workspace`, the generic transport binds the session to that one concrete workspace and removes redundant workspace selection from the client-facing surface where applicable.

Workspace containers (`wsc_*`) remain orchestration groups and are never substituted for a concrete filesystem workspace.

See [Workspaces](workspaces.md) for the canonical workspace model.

## Upstream MCP aggregation

`chatgpt-mcp` can connect to other MCP servers and expose selected upstream tools through its own catalog.

Start with:

```bash
cgm upstream --help
cgm upstream server --help
```

Common operations:

```bash
cgm upstream server list
cgm upstream server show <id>
cgm upstream server status <id>
cgm upstream server tools <id>
cgm upstream server enable <id>
cgm upstream server disable <id>
cgm upstream server remove <id>
```

### HTTP upstream

```bash
cgm upstream server add example \
  --transport http \
  --url https://mcp.example.com/mcp \
  --auth auto \
  --expose all
```

### stdio upstream

```bash
cgm upstream server add local-tools \
  --transport stdio \
  --command node \
  --arg /path/to/server.mjs \
  --cwd /path/to/project \
  --expose all
```

Tool exposure can be narrowed with prefixes, allowlists, disabled-tool lists, or exposure modes. Use `cgm upstream server add --help` and `configure --help` for the installed version's exact fields.

`cgm mcp server ...` is a deprecated compatibility path; new automation should use `cgm upstream server ...`.

HTTP upstreams authenticate with configured headers and optional bearer-token environment variables. Leftover `auth: {type: oauth|auto|none}` fields in saved config are ignored.

## Outbound network policy

HTTP upstreams are subject to outbound URL and redirect validation. Public non-loopback targets normally require HTTPS; private/link-local/metadata destinations are rejected unless that upstream explicitly opts into private-network access.

Use `--allow-private-network` only for upstreams you intentionally expect to reach on loopback/private networks.

See [Security](security.md#upstream-http-outbound-policy) for the exact boundary.

## Tool catalog changes

The visible tool catalog can change when upstream servers are added, removed, enabled, disabled, or rediscovered, or when local feature/configuration state changes the available tool surface.

Replacement discovery is applied as a complete catalog update rather than intentionally exposing a partially refreshed upstream.

## Protocol profile

The integrated ChatGPT runtime follows the project's current stateless MCP profile and OpenAI tunnel requirements. Generic `cgm mcp stdio` / `cgm mcp http` transports provide standards-compatible client lifecycles for ordinary MCP clients.

The current binary is the authoritative source for its supported transport/command surface:

```bash
cgm mcp --help
cgm mcp stdio --help
cgm mcp http --help
```

Protocol-specific implementation details such as the exact revision, method/header validation, MRTR support, and compatibility behavior are intentionally kept out of the normal setup path because most users do not need them to connect or operate the runtime.
