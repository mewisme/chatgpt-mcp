# plugins/cf-tunnel/internal/cloudflared

Adapted Cloudflare Quick Tunnel client for exposing a single local HTTP origin.

This package must stay independent of `chatgpt-mcp/internal`. Application
lifecycle, auth gates, CLI, TUI, and the `cf-tunnel` plugin live outside this
directory.

See `UPSTREAM.md` for the pinned `cloudflare/cloudflared` tag, license, and
extraction boundaries. Do not import `github.com/cloudflare/cloudflared/...`
from this module.

```go
tun, err := cloudflared.Start(ctx, cloudflared.Config{OriginURL: "http://127.0.0.1:37421"})
```

Context cancellation is shutdown. `Wait` blocks until the supervisor exits.
