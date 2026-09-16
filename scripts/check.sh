#!/usr/bin/env bash
# Fast local quality checks (subset of CI). Usage: ./scripts/check.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

default_config_dir="${HOME}/.config/chatgpt-mcp"
if [[ -z "${CHATGPT_MCP_CONFIG_DIR:-}" ]]; then
  CHATGPT_MCP_CONFIG_DIR="$(mktemp -d)"
  export CHATGPT_MCP_CONFIG_DIR
  trap 'rm -rf "${CHATGPT_MCP_CONFIG_DIR}"' EXIT
else
  export CHATGPT_MCP_CONFIG_DIR
  if [[ "$(realpath -m "${CHATGPT_MCP_CONFIG_DIR}")" == "$(realpath -m "${default_config_dir}")" ]]; then
    echo "refusing to run checks with CHATGPT_MCP_CONFIG_DIR set to the live default config root: ${CHATGPT_MCP_CONFIG_DIR}" >&2
    exit 1
  fi
fi

echo "==> gofmt"
test -z "$(gofmt -l . | tee /dev/stderr)"

echo "==> go vet"
go vet ./...

echo "==> staticcheck"
if ! command -v staticcheck >/dev/null 2>&1; then
  go install honnef.co/go/tools/cmd/staticcheck@v0.8.1
fi
staticcheck ./...

echo "==> go mod tidy -diff"
go mod tidy -diff

echo "==> go test (short packages smoke)"
go test ./internal/outboundpolicy/ ./internal/approval/ ./internal/config/ ./internal/secretstore/ -count=1

echo "==> shellcheck install.sh"
if command -v shellcheck >/dev/null 2>&1; then
  shellcheck install.sh
else
  echo "skip: shellcheck not installed"
fi

echo "==> admin-ui lint/typecheck (if pnpm available)"
if command -v pnpm >/dev/null 2>&1 && [[ -d plugins/admin-ui/node_modules ]]; then
  pnpm --dir plugins/admin-ui lint
  pnpm --dir plugins/admin-ui typecheck
else
  echo "skip: pnpm or plugins/admin-ui/node_modules missing"
fi

echo "OK: local checks passed"
