import { Activity, Cloud, FolderGit2, Home, Server, Settings, Wrench, type LucideIcon } from "lucide-react"

export type NavItem = { id: string; path: string; title: string; description: string; icon: LucideIcon }

export const adminAppTitle = "ChatGPT MCP"

export function adminDocumentTitle(title: string) { return `${title} | ${adminAppTitle}` }

export const navItems: NavItem[] = [
  { id: "overview", path: "/overview", title: "Overview", description: "Runtime health and configuration at a glance.", icon: Home },
  { id: "workspaces", path: "/workspaces", title: "Workspaces", description: "Manage canonical project roots and workspace handles.", icon: FolderGit2 },
  { id: "tools", path: "/tools", title: "Tools", description: "Inspect the tools currently exposed by this runtime.", icon: Wrench },
  { id: "servers", path: "/servers", title: "MCP Servers", description: "Configure upstream MCP servers, health, tools, and OAuth.", icon: Server },
  { id: "tunnel", path: "/tunnel", title: "Tunnel", description: "Configure and monitor the OpenAI Secure MCP Tunnel.", icon: Cloud },
  { id: "activity", path: "/activity", title: "Activity", description: "Watch live MCP requests and tool execution events.", icon: Activity },
  { id: "settings", path: "/settings", title: "Settings", description: "Configure listeners, presets, and authentication.", icon: Settings },
]

export function adminNavItemFromPath(pathname: string) {
  const normalized = pathname === "/" || pathname === "/index.html" ? navItems[0].path : pathname.replace(/\/+$/, "") || navItems[0].path
  return navItems.find((item) => item.path === normalized) ?? navItems[0]
}
