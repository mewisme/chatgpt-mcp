# Contributing to chatgpt-mcp

Thanks for contributing. This guide covers how to propose changes; detailed build, test, CI, and release steps live in [docs/development.md](docs/development.md).

By participating, you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Before you start

- Search [existing issues](https://github.com/mewisme/chatgpt-mcp/issues) and PRs to avoid duplicates.
- For security vulnerabilities, do **not** open a public issue. Follow [SECURITY.md](SECURITY.md).
- Prefer a focused PR that does one thing well over a large mixed change.

## Development setup

Requirements:

- Go 1.27+
- Node.js 24+
- pnpm 11+

Quick path:

```bash
pnpm --dir plugins/admin-ui install
pnpm --dir plugins/admin-ui build
CHATGPT_MCP_CONFIG_DIR="$(mktemp -d)" go test ./...
go build -trimpath ./
```

Production Admin UI assets are distributed as the independent `admin-ui` plugin. Use `node scripts/prepare-web-embed.mjs --from-dist` when you need to package the current frontend build.

Fast local gate (subset of CI):

```bash
./scripts/check.sh
```

Tests must never use the real default/global config directory. Always isolate with `CHATGPT_MCP_CONFIG_DIR` or `--config-dir`. See [docs/development.md](docs/development.md).

## Pull requests

1. Fork and branch from `main`.
2. Keep changes scoped; update docs when behavior or UX changes.
3. Fill out the PR template.
4. Ensure CI is green.

Suggested local checks before opening a PR:

```bash
./scripts/check.sh
CHATGPT_MCP_CONFIG_DIR="$(mktemp -d)" go test ./...
go vet ./...
pnpm --dir plugins/admin-ui test
pnpm --dir plugins/admin-ui lint
pnpm --dir plugins/admin-ui typecheck
pnpm --dir plugins/admin-ui build
```

For changes that affect services, tunnel connectivity, runtime logs, configuration, or MCP protocol behavior, also run the release smoke described in [docs/development.md](docs/development.md).

## Issues

Use the GitHub issue templates:

- **Bug report** — include OS, `cgm version`, repro steps, expected vs actual
- **Feature request** — describe the problem, proposal, and alternatives

Questions about product security boundaries belong in discussion or docs issues; vulnerability reports belong in [SECURITY.md](SECURITY.md).

## Releases and changelog

Releases are cut from tags on `main` via GoReleaser. Release notes live on [GitHub Releases](https://github.com/mewisme/chatgpt-mcp/releases); there is no separate root `CHANGELOG.md`.

## License

Contributions are licensed under the project [Apache License 2.0](LICENSE).

### Copied or adapted source

If you add third-party source to the tree, keep the upstream license and record:

- upstream URL and exact revision
- original license file
- NOTICE/attribution and any trademark limits
- a short provenance file (see `plugins/cf-tunnel/internal/cloudflared/UPSTREAM.md`)
- local modifications

Do not relicense someone else's code as Apache-2.0. Point the matching plugin or core artifact at that material in `internal/licenseinventory` so NOTICE/SBOM generation stays attached to the artifact that ships it.

### New dependency licenses

Runtime and distributed dependencies must use an SPDX identifier already listed in `licenses/policy.json`. Adding a new license family requires a reviewable allowlist entry with `kind` and, for copyleft/custom/restrictive licenses, a rationale and optional `artifacts` scope. Unknown or unallowlisted licenses fail `go test ./internal/licenseinventory` and `go run ./internal/licenseinventory/cmd/license-inventory`.
