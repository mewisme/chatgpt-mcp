# pkg/cloudflared

Adapted Cloudflare Quick Tunnel client for exposing a single local HTTP origin.

This package must stay independent of `chatgpt-mcp/internal`. Application
lifecycle, auth gates, CLI, TUI, and the `cf-tunnel` plugin live outside this
directory.

See `UPSTREAM.md` for the pinned `cloudflare/cloudflared` tag, license, and
extraction boundaries. Do not import `github.com/cloudflare/cloudflared/...`
from this module.
