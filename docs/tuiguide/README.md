# Embedded TUI Guide

These Markdown files are the canonical feature guides for the `cgm tui` Command Center. They are also embedded directly into the Go binary by `guides.go`; do not maintain a separate copy of the same guide text in Go source.

| Topic | File |
| --- | --- |
| Getting started and navigation | [Getting Started](content/getting-started.md) |
| Editors, dirty drafts, path pickers, and switches | [Editors & Forms](content/editors.md) |
| Workspaces, containers, and Project Context | [Workspaces](content/workspaces.md) |
| Upstream MCP servers and OAuth | [MCP Servers](content/mcp.md) |
| Runtime and managed tunnels | [Tunnel](content/tunnel.md) |
| Approval requests and live approval dialogs | [Requests & Approvals](content/requests.md) |
| Runtime logs and command execution | [Logs](content/logs.md) |
| Typed configuration and storage operations | [Configuration](content/config/index.md) |
| Global Context, rules, and sources | [Instruction](content/instruction.md) |
| Runtime/service/auth/install/update operations | [Runtime & System](content/runtime.md) |

The canonical guide tree lives under `content/`. A leaf may be `child.md`; a branch is a directory with `index.md`, and branches may nest without a fixed depth. Inside the TUI, use `Ctrl+K` → **Guide** to browse these topics, or open a direct guide action. `cgm tui guide <topic...>` deep-links to any node. Only the selected Markdown file is rendered by Glamour.