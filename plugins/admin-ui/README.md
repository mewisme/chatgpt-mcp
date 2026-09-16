# Admin UI plugin

Official static Admin UI bundle for ChatGPT MCP.

The plugin provides `web-ui/admin` only. The core process continues to own the admin listener, authentication, API routes, activity endpoints, and security headers. This plugin contains no backend handlers and requests no runtime permissions.

React source, Vite/Tailwind/shadcn configuration, frontend tests, and the production `dist/` live in this directory. The release workflow builds that frontend, packages `dist/` as one deterministic platform-independent ZIP, computes its SHA-256 digest, and publishes the signed manifest and artifact to the official plugin marketplace.

## Requirements

- Node.js 24+
- pnpm 11+

## Commands

```bash
pnpm --dir plugins/admin-ui install
pnpm --dir plugins/admin-ui test
pnpm --dir plugins/admin-ui lint
pnpm --dir plugins/admin-ui typecheck
pnpm --dir plugins/admin-ui build
```

Local Vite dev server:

```bash
pnpm --dir plugins/admin-ui dev
```

## Package the plugin artifact

From the repository root:

```bash
node scripts/prepare-web-embed.mjs
```

The helper builds `plugins/admin-ui/dist` and packages it under `dist/plugins`. Use `--no-deps` to reuse the current frontend installation, or `--from-dist` to package an existing `plugins/admin-ui/dist`.

## Adding shadcn components

```bash
pnpm --dir plugins/admin-ui dlx shadcn@latest add button
```

UI primitives live under `src/components/ui/`.

Full backend/frontend workflow, CI gates, and release notes: [docs/development.md](../../docs/development.md).
