#!/usr/bin/env bash
set -euo pipefail

echo "Remove built-in web dist"
rm -rf internal/web/dist
echo "Recreate built-in web dist directory"
mkdir -p internal/web/dist
echo "Build web"
pnpm --dir web build
echo "Copy"
cp -r web/dist/* internal/web/dist/
