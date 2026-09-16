#!/usr/bin/env bash
# First-party staticcheck. Adapted cloudflared source keeps upstream style.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if ! command -v staticcheck >/dev/null 2>&1; then
  go install honnef.co/go/tools/cmd/staticcheck@v0.8.1
fi

mapfile -t pkgs < <(go list ./... | grep -vE '/pkg/cloudflared($|/)')
staticcheck "${pkgs[@]}"
