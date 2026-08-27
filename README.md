# chatgpt-mcp

A self-hosted MCP server with an integrated admin dashboard, tool runtime, upstream MCP management, activity observability, and optional tunnel support.

## Features

- MCP JSON-RPC server
- MCP session lifecycle support
- Tool registry and execution runtime
- Upstream MCP server management
- Admin web dashboard
- Activity stream and observability
- Token based authentication
- Tunnel process integration
- Single binary distribution (planned web embedding)

## Installation

### Download release

Download the latest binary from GitHub Releases:

```bash
chatgpt-mcp
```

### Build from source

Requirements:

- Go 1.27+
- Node.js 24+
- pnpm 11+

Build web assets:

```bash
cd web
pnpm install
pnpm build
```

Build binary:

```bash
go build -o chatgpt-mcp .
```

## First run

Initialize configuration and create authentication tokens:

```bash
chatgpt-mcp init
```

This creates:

```
~/.config/chatgpt-mcp/config.json
```

Start server:

```bash
chatgpt-mcp serve
```

Default endpoint:

```
http://127.0.0.1:3000
```

## Authentication

Create or rotate tokens:

```bash
chatgpt-mcp auth mcp-create
chatgpt-mcp auth admin-create
```

Use the token as:

```
Authorization: Bearer <token>
```

## CLI

```text
chatgpt-mcp
├── serve
├── init
├── auth
│   ├── mcp-create
│   └── admin-create
├── config
├── mcp
└── tunnel
```

## Tunnel

Configure a tunnel process:

```bash
chatgpt-mcp tunnel configure \
  --enabled \
  --command cloudflared
```

Check status:

```bash
chatgpt-mcp tunnel status
```

## Development

Run backend:

```bash
go run . serve
```

Run frontend:

```bash
cd web
pnpm dev
```

## Release

Releases are created automatically when pushing a version tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

GitHub Actions runs tests and GoReleaser to publish binaries.

## License

MIT
