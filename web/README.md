# Admin UI (`web/`)

React admin dashboard for `chatgpt-mcp`, built with React, TypeScript, Vite, Tailwind CSS, and shadcn/ui. Production assets are distributed independently as the official `admin-ui` plugin; they are not embedded in the Go binary.

## Requirements

- Node.js 24+
- pnpm 11+

## Commands

```bash
pnpm --dir web install
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web typecheck
pnpm --dir web build
```

Local Vite dev server:

```bash
pnpm --dir web dev
```

## Build the Admin UI plugin

From the repository root:

```bash
node scripts/prepare-web-embed.mjs
```

The compatibility helper builds `web/dist` and packages it as a deterministic platform-independent `admin-ui` plugin artifact under `dist/plugins`. It no longer copies assets into the Go source tree. Use `--no-deps` to reuse the current frontend installation, or `--from-dist` to package an existing `web/dist`.

The core continues to own the admin listener, authentication, `/api/*`, OAuth callback, activity endpoints, and security headers. The plugin only provides signed static assets through `web-ui/admin` and requests no runtime permissions.

Full backend/frontend workflow, CI gates, and release notes: [docs/development.md](../docs/development.md).

## Adding shadcn components

```bash
pnpm --dir web dlx shadcn@latest add button
```

UI primitives live under `src/components/ui/`.
