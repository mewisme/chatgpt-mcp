import { matchPath } from "react-router-dom"
import { adminNavItemFromPath, navItems } from "@/lib/admin-navigation"

export type AdminRouteID = "overview" | "workspaces" | "requests" | "tools" | "servers" | "tunnel" | "activity" | "activity-call" | "settings" | "workspace" | "workspace-context" | "workspace-requests" | "workspace-activity" | "workspace-global"
export type AdminRoute = { id: AdminRouteID; navID: string; path: string; title: string; description: string; workspaceID?: string; callID?: string }

const workspaceRoutes: Record<string, { id: AdminRouteID; title: string; description: string }> = {
  context: { id: "workspace-context", title: "Project Context", description: "Preview the effective instruction context for this workspace." },
  requests: { id: "workspace-requests", title: "Workspace Requests", description: "Review control approval requests scoped to this workspace." },
  activity: { id: "workspace-activity", title: "Workspace Activity", description: "Inspect activity and command executions scoped to this workspace." },
}

export function adminRouteFromPath(pathname: string): AdminRoute {
  const normalized = normalizePath(pathname)
  const staticItem = navItems.find((item) => item.path === normalized)
  if (staticItem) return { id: staticItem.id as AdminRouteID, navID: staticItem.id, path: staticItem.path, title: staticItem.title, description: staticItem.description }
  const activityCall = matchPath("/activity/:callID", normalized)
  if (activityCall?.params.callID) return { id: "activity-call", navID: "activity", path: normalized, title: "Tool Call", description: "Inspect one tool call and its complete runtime metadata.", callID: activityCall.params.callID }
  const workspace = matchPath("/workspaces/:workspaceID/:child?", normalized)
  if (workspace?.params.workspaceID) {
    const workspaceID = workspace.params.workspaceID
    const child = workspace.params.child
    if (!child) return { id: "workspace", navID: "workspaces", path: normalized, title: "Workspace", description: "Inspect this registered workspace and its workspace-scoped tools.", workspaceID }
    const childRoute = workspaceRoutes[child]
    if (childRoute) return { ...childRoute, navID: "workspaces", path: normalized, workspaceID }
  }
  const fallback = adminNavItemFromPath("/overview")
  return { id: fallback.id as AdminRouteID, navID: fallback.id, path: fallback.path, title: fallback.title, description: fallback.description }
}

function normalizePath(pathname: string) {
  if (pathname === "/" || pathname === "/index.html") return "/overview"
  return pathname.replace(/\/+$/, "") || "/overview"
}