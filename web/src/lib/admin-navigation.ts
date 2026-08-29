import { Activity, Cloud, FolderGit2, Home, Server, Settings, Wrench, type LucideIcon } from "lucide-react"

export type NavItem = { id: string; title: string; description: string; icon: LucideIcon }

export const navItems: NavItem[] = [
  { id: "overview", title: "Overview", description: "Runtime health and configuration at a glance.", icon: Home },
  { id: "workspaces", title: "Workspaces", description: "Manage canonical project roots and workspace handles.", icon: FolderGit2 },
  { id: "tools", title: "Tools", description: "Inspect the tools currently exposed by this runtime.", icon: Wrench },
  { id: "servers", title: "MCP Servers", description: "Configure upstream MCP servers, health, tools, and OAuth.", icon: Server },
  { id: "tunnel", title: "Tunnel", description: "Configure and monitor the OpenAI Secure MCP Tunnel.", icon: Cloud },
  { id: "activity", title: "Activity", description: "Watch live MCP requests and tool execution events.", icon: Activity },
  { id: "settings", title: "Settings", description: "Configure listeners, presets, and authentication.", icon: Settings },
]
