# chatgpt-mcp

A self-hosted MCP server with an embedded admin dashboard, authentication, upstream MCP management, activity observability and optional tunnel integration.

## Features

- MCP protocol runtime
- JSON-RPC validation and session lifecycle
- Tool registry and upstream MCP servers
- Web admin dashboard
- Token based authentication
- Activity stream and audit events
- Managed tunnel process
- Single binary distribution target

## Installation

Download the latest release from GitHub Releases.

### Build from source

Requirements:

- Go 1.27+
- Node.js 24+
- pnpm 11+

```bash
pnpm --dir web install
pnpm --dir web build
go build .
```

## First run

Initialize configuration:

```bash
chatgpt-mcp init
```

Start server:

```bash
chatgpt-mcp serve
```

Open:

```
http://127.0.0.1:3000
```

## Authentication

Create tokens:

```bash
chatgpt-mcp auth mcp-create
chatgpt-mcp auth admin-create
```

Tokens are only printed once. Only hashes are stored in configuration.

Use:

```http
Authorization: Bearer <token>
```

## CLI

```text
chatgpt-mcp
├── serve
├── init
├── auth
├── config
├── mcp
└── tunnel
```

## Tunnel

Configure a managed tunnel process:

```bash
chatgpt-mcp tunnel configure --command cloudflared --arg tunnel --arg run
chatgpt-mcp tunnel enable
```

Check status:

```bash
chatgpt-mcp tunnel status
```

## Development

Run checks locally:

```bash
go test ./...
go vet ./...

cd web
pnpm lint
pnpm typecheck
pnpm build
```

## CI

Pull requests and pushes to `main` run:

- Go tests
- Go vet
- Web lint
- Web typecheck
- Web production build
- Cross platform Go builds

## Release

Create a tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

GitHub Actions runs GoReleaser and publishes release artifacts.

## License

MIT
