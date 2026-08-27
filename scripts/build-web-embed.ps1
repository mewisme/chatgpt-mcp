$ErrorActionPreference = 'Stop'
pnpm --dir web build
node scripts/prepare-web-embed.mjs
