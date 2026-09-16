# CF Tunnel plugin

Official first-party runtime plugin that exposes local MCP and Admin HTTP through Cloudflare Quick Tunnels.

This plugin is installed independently of the `cgm` core binary:

```text
cgm plugin install cf-tunnel
cgm tunnel cf start mcp
```

The packaged entrypoint speaks the bounded NDJSON runtime-plugin protocol. Core owns origin/auth gating; this plugin owns Quick Tunnel transport and reconnect behavior.
