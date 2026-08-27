#!/usr/bin/env bash
set -euo pipefail

rm -rf internal/web/dist
mkdir -p internal/web/dist
cp -r web/dist/* internal/web/dist/
