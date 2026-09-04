# Documentation

This directory contains the detailed documentation for `chatgpt-mcp`. The repository README is intentionally short and optimized as a project landing page.

## Start here

| Goal | Guide |
| --- | --- |
| Install and start `chatgpt-mcp` | [Getting started](getting-started.md) |
| Connect ChatGPT to a private/local runtime | [OpenAI + ChatGPT setup](openai-chatgpt.md) |
| Run as a foreground process or managed service | [Runtime and services](runtime.md) |
| Configure ports, auth, exposure, formats, and workspaces | [Configuration](configuration.md) |
| Browse commands, interactive TUI support, and common flag combinations | [CLI reference](cli-reference.md) |
| Understand MCP protocol behavior and upstream servers | [MCP and upstreams](mcp.md) |
| Review filesystem, shell, auth, tunnel, and control-plane boundaries | [Security](security.md) |
| Build, test, run smoke tests, CI, and release | [Development](development.md) |
| Diagnose common failures | [Troubleshooting](troubleshooting.md) |

## Recommended reading paths

### I just want ChatGPT connected to my computer

1. [Getting started](getting-started.md)
2. [OpenAI + ChatGPT setup](openai-chatgpt.md)
3. [Runtime and services](runtime.md)
4. [Security](security.md)

### I am operating a remote Linux server

1. [Getting started](getting-started.md)
2. [Runtime and services](runtime.md) — especially user vs system service behavior
3. [OpenAI + ChatGPT setup](openai-chatgpt.md)
4. [Troubleshooting](troubleshooting.md)

### I use multiple ChatGPT conversations

1. [MCP and upstreams](mcp.md) — MCP session behavior
2. [Security](security.md) — session-to-workspace isolation
3. [Configuration](configuration.md) — workspace registration and stable IDs

### I am developing or contributing

1. [MCP and upstreams](mcp.md)
2. [Configuration](configuration.md)
3. [Security](security.md)
4. [Development](development.md)

## External references

The OpenAI setup guide is based on the current official documentation. OpenAI product UI and availability can change, so use these as the authoritative references if the UI differs from the screenshots or wording you see locally:

- [OpenAI Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)
- [Developer mode and MCP apps in ChatGPT](https://help.openai.com/en/articles/12584461)
- [OpenAI tunnel-client](https://github.com/openai/tunnel-client)
- [Platform tunnel settings](https://platform.openai.com/settings/organization/tunnels)
- [Platform runtime API keys](https://platform.openai.com/settings/organization/api-keys)
- [ChatGPT app/connector settings](https://chatgpt.com/#settings/Connectors)
