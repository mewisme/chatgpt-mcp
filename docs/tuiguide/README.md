# Embedded TUI Guide

These Markdown files are the canonical feature guides for the `cgm tui` Command Center. They are also embedded directly into the Go binary by `guides.go`; do not maintain a separate copy of the same guide text in Go source.

| Topic | File |
| --- | --- |
| Getting started and navigation | [Getting Started](getting-started.md) |
| Editors, dirty drafts, path pickers, and switches | [Editors & Forms](editors.md) |
| Workspaces, containers, and Project Context | [Workspaces](workspaces.md) |
| Upstream MCP servers and OAuth | [MCP Servers](mcp.md) |
| Runtime and managed tunnels | [Tunnel](tunnel.md) |
| Approval requests and live approval dialogs | [Requests & Approvals](requests.md) |
| Runtime logs and command execution | [Logs](logs.md) |
| Typed configuration and storage operations | [Configuration](config.md) |
| Global Context, rules, and sources | [Instruction](instruction.md) |
| Runtime/service/auth/install/update operations | [Runtime & System](runtime.md) |

Inside the TUI, use `Ctrl+K` → **Guide** to browse these topics, or open a direct guide action. `cgm tui guide <topic>` deep-links to one topic. Only the selected Markdown file is rendered by Glamour.