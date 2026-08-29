# chatgpt-mcp

[![CI](https://github.com/mewisme/chatgpt-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/mewisme/chatgpt-mcp/actions/workflows/ci.yml)
[![Release](https://github.com/mewisme/chatgpt-mcp/actions/workflows/release.yml/badge.svg)](https://github.com/mewisme/chatgpt-mcp/actions/workflows/release.yml)

A self-hosted MCP 2026-07-28 runtime written in Go with an embedded React administration dashboard and a builtin OpenAI Secure MCP Tunnel client.

`chatgpt-mcp` provides one cross-platform binary for local workspace tools, upstream MCP aggregation, OpenAI tunnel connectivity, authentication, activity monitoring, and administration.

## Features

- Stateless MCP 2026-07-28 HTTP runtime at `/mcp`
- `server/discover`, `tools/list`, `tools/call`, and `subscriptions/listen`
- Per-request protocol version, client capabilities, and client metadata
- SEP-2243 `Mcp-Method`, `Mcp-Name`, and `Mcp-Param-*` validation
- Multi Round-Trip Requests (MRTR) with `input_required`, `inputRequests`, `inputResponses`, and `requestState`
- Workspace-bound filesystem, shell, Git, context, rules, skills, checkpoint, and utility tools
- Dynamic upstream MCP server aggregation with OAuth and MRTR relay
- Builtin OpenAI Secure MCP Tunnel using `github.com/openai/tunnel-client`
- Embedded React admin dashboard
- MCP and admin bearer-token authentication
- Activity stream and audit events
- Cross-platform single-binary releases for Linux, Windows, and macOS

## Requirements

For source builds:

- Go 1.27+
- Node.js 24+
- pnpm 11+

Binary releases do not require Node.js or pnpm.

## Installation

### Local source install

From a cloned repository:

```bash
node scripts/install-local.mjs
```

The installer restores frontend dependencies, builds the dashboard, prepares `internal/web/dist`, then runs:

```bash
go install .
```

Useful variants:

```bash
node scripts/install-local.mjs --no-deps
node scripts/install-local.mjs --from-dist
```

### Binary release

Download a release artifact for one of these targets:

- Linux amd64/arm64
- Windows amd64/arm64
- macOS amd64/arm64

Then run:

```bash
chatgpt-mcp --version
```

## Quick start

Initialize the local configuration:

```bash
chatgpt-mcp init
```

The command creates MCP/admin credentials and stores configuration under:

```text
~/.config/chatgpt-mcp/
```

Start the runtime:

```bash
chatgpt-mcp serve
```

Default endpoints:

```text
MCP:   http://127.0.0.1:37421/mcp
Admin: http://127.0.0.1:37422/
```

Inspect the current runtime configuration:

```bash
chatgpt-mcp status
chatgpt-mcp config get
```

Remove all local `chatgpt-mcp` configuration and state:

```bash
chatgpt-mcp uninit
```

## MCP protocol

The HTTP endpoint implements the stateless MCP `2026-07-28` protocol revision.

Modern requests use `POST /mcp` and carry routing/version metadata on every request. A typical request includes:

```http
MCP-Protocol-Version: 2026-07-28
Mcp-Method: tools/list
Content-Type: application/json
```

with request metadata:

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

There is no `initialize` handshake and no `Mcp-Session-Id` in this protocol revision. Removed or unknown MCP methods return HTTP `404` with JSON-RPC error `-32601`. `GET` and `DELETE` on the MCP endpoint return `405`.

## Authentication

`chatgpt-mcp` stores token hashes rather than plaintext MCP/admin tokens. Tokens are shown when they are created or rotated.

Create or rotate credentials:

```bash
chatgpt-mcp auth mcp-create
chatgpt-mcp auth admin-create
```

Use an enabled token with:

```http
Authorization: Bearer <token>
```

Authentication can be inspected with:

```bash
chatgpt-mcp auth status
```

## OpenAI Secure MCP Tunnel

The OpenAI Secure MCP Tunnel is embedded directly in the Go binary. No `cloudflared`, `ngrok`, external tunnel executable, or public local MCP port is required.

Configure it with a tunnel ID and Runtime API key:

```bash
chatgpt-mcp tunnel configure \
  --enabled \
  --id tunnel_... \
  --api-key <runtime-api-key>
```

Optional OpenAI-specific settings:

```bash
chatgpt-mcp tunnel configure \
  --control-plane-base-url https://api.openai.com \
  --organization-id org_...
```

Check configuration/runtime status:

```bash
chatgpt-mcp tunnel status
```

Enable or disable automatic tunnel startup:

```bash
chatgpt-mcp tunnel enable
chatgpt-mcp tunnel disable
```

Run only the builtin tunnel in the foreground:

```bash
chatgpt-mcp tunnel run
```

The tunnel API key is kept separately from `config.json` in:

```text
~/.config/chatgpt-mcp/tunnel.json
```

## Upstream MCP servers

`chatgpt-mcp` can connect to other MCP servers and expose enabled upstream tools through the same local runtime.

Management is available through:

```bash
chatgpt-mcp mcp --help
```

and the admin dashboard.

Upstream HTTP OAuth credentials are stored separately from normal runtime configuration. Upstream tool changes propagate into the exposed tool catalog and MCP subscriptions.

## Workspaces

Filesystem, shell, and Git mutations are workspace-bound. Register a workspace before using tools that operate on local project files:

```bash
chatgpt-mcp workspace --help
```

Workspace handles are explicit and immutable; tool calls cannot silently switch to another workspace.

## CLI

```text
chatgpt-mcp
├── serve
├── init
├── uninit
├── status
├── config
├── auth
├── workspace
├── mcp
└── tunnel
```

Use `--help` on any command for the current flags and subcommands.

## Development

Install frontend dependencies:

```bash
pnpm --dir web install
```

Run frontend checks:

```bash
pnpm --dir web lint
pnpm --dir web typecheck
pnpm --dir web build
```

Prepare the ignored Go embed directory before compiling the backend:

```bash
node scripts/prepare-web-embed.mjs
```

Run backend verification:

```bash
go mod verify
go test ./...
go vet ./...
go build -trimpath ./
```

Run the native release smoke against a built binary:

```bash
go build -trimpath -o chatgpt-mcp ./
node scripts/smoke-release.mjs ./chatgpt-mcp
```

The smoke uses an isolated temporary home directory and verifies init/config/status, HTTP health, MCP discovery, tool listing, modern error behavior, shutdown, and uninit.

## CI

Pushes to `main` and pull requests run:

- Web lint, typecheck, and production build
- Native tests, vet, build, local-install smoke, and runtime/MCP E2E smoke on Linux, Windows, and macOS
- Cross-build validation for every release target:
  - linux/amd64
  - linux/arm64
  - windows/amd64
  - windows/arm64
  - darwin/amd64
  - darwin/arm64

Tag releases repeat the web and native gates before GoReleaser is allowed to publish.

## Release

Releases are produced by GoReleaser only after all native release jobs pass.

The repository is currently suitable for an initial `v0.1.0` release while the public API remains pre-1.0.

After the final release-readiness commit is on `main` and CI is green:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Release archives contain the standalone binary with the admin dashboard embedded, plus `checksums.txt`.

## License

MIT License. Copyright (c) 2026 Mew.
