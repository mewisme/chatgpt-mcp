import { lazy, Suspense, useEffect, useState } from "react"
import { LoaderCircle } from "lucide-react"
import { AppSidebar } from "@/components/app-sidebar"
import { RequestApprovalHost } from "@/components/request-approval-host"
import { PageLoading } from "@/components/page-state"
import { adminDocumentTitle, navItems } from "@/lib/admin-navigation"
import { adminRouteFromPath, type AdminRoute } from "@/lib/admin-route"
import { AdminRouterProvider } from "@/lib/admin-router-provider"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import { adminApi, adminToken, ApiError } from "@/lib/api"

const ActivityPage = lazy(() => import("@/pages/activity").then((module) => ({ default: module.ActivityPage })))
const LoginPage = lazy(() => import("@/pages/login").then((module) => ({ default: module.LoginPage })))
const OverviewPage = lazy(() => import("@/pages/overview").then((module) => ({ default: module.OverviewPage })))
const RequestsPage = lazy(() => import("@/pages/requests").then((module) => ({ default: module.RequestsPage })))
const ServersPage = lazy(() => import("@/pages/servers").then((module) => ({ default: module.ServersPage })))
const SettingsPage = lazy(() => import("@/pages/settings").then((module) => ({ default: module.SettingsPage })))
const ToolsPage = lazy(() => import("@/pages/tools").then((module) => ({ default: module.ToolsPage })))
const TunnelPage = lazy(() => import("@/pages/tunnel").then((module) => ({ default: module.TunnelPage })))
const WorkspacesPage = lazy(() => import("@/pages/workspaces").then((module) => ({ default: module.WorkspacesPage })))
const WorkspacePage = lazy(() => import("@/pages/workspace").then((module) => ({ default: module.WorkspacePage })))

const pages: Record<string, React.ComponentType> = {
  overview: OverviewPage,
  workspaces: WorkspacesPage,
  requests: RequestsPage,
  tools: ToolsPage,
  servers: ServersPage,
  tunnel: TunnelPage,
  activity: ActivityPage,
  settings: SettingsPage,
  workspace: WorkspacePage,
  "workspace-context": WorkspacePage,
  "workspace-requests": WorkspacePage,
  "workspace-activity": WorkspacePage,
  "workspace-global": WorkspacesPage,
}

export function App() {
  const [route, setRoute] = useState<AdminRoute>(() => adminRouteFromPath(window.location.pathname))
  const [authenticated, setAuthenticated] = useState<boolean | null>(null)
  const [authRequired, setAuthRequired] = useState(true)
  const page = route.navID
  const Page = pages[route.id] ?? OverviewPage

  useEffect(() => {
    const sync = () => {
      const current = adminRouteFromPath(window.location.pathname)
      if (window.location.pathname !== current.path) window.history.replaceState(window.history.state, "", current.path)
      setRoute(current)
    }
    sync()
    window.addEventListener("popstate", sync)
    return () => window.removeEventListener("popstate", sync)
  }, [])

  useEffect(() => {
    void adminApi.health().then((health) => { setAuthRequired(health.auth_enabled); setAuthenticated(true) }).catch((value) => {
      if (value instanceof ApiError && value.status === 401) { adminToken.clear(); setAuthRequired(true); setAuthenticated(false); return }
      setAuthenticated(false)
    })
  }, [])

  useEffect(() => { document.title = adminDocumentTitle(authenticated === null ? "Connecting" : authenticated ? route.title : "Login") }, [authenticated, route.title])

  if (authenticated === null) return <div className="flex min-h-screen items-center justify-center gap-2 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" />Connecting to admin API...</div>
  if (!authenticated) return <Suspense fallback={<FullPageLoading label="Loading sign in..." />}><LoginPage onAuthenticated={() => { setAuthRequired(true); setAuthenticated(true) }} /></Suspense>

  function navigate(next: string) {
    const item = navItems.find((item) => item.id === next) ?? navItems[0]
    navigatePath(item.path)
  }
  function navigatePath(path: string, options?: { replace?: boolean }) { const next = adminRouteFromPath(path); if (window.location.pathname !== next.path) window.history[options?.replace ? "replaceState" : "pushState"](window.history.state, "", next.path); setRoute(next) }
  function signOut() { adminToken.clear(); setAuthenticated(false) }

  return <AdminRouterProvider route={route} navigate={navigatePath}><TooltipProvider><SidebarProvider><AppSidebar authRequired={authRequired} page={page} onPageChange={navigate} onSignOut={signOut} /><SidebarInset className="min-w-0"><header className="sticky top-0 z-20 flex min-h-14 items-center gap-3 border-b bg-background/95 px-3 backdrop-blur supports-[backdrop-filter]:bg-background/80 sm:px-4"><SidebarTrigger /><div className="min-w-0 flex-1"><div className="truncate text-sm font-semibold">{route.title}</div><div className="hidden truncate text-xs text-muted-foreground sm:block">{route.description}</div></div></header><div className="min-w-0 flex-1 bg-muted/20"><div className="mx-auto w-full max-w-[1400px] p-4 sm:p-6 lg:p-8"><Suspense fallback={<PageLoading rows={6} />}><Page key={route.path} /></Suspense></div></div></SidebarInset><RequestApprovalHost /><Toaster /></SidebarProvider></TooltipProvider></AdminRouterProvider>
}

function FullPageLoading({ label }: { label: string }) { return <div className="flex min-h-screen items-center justify-center gap-2 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" />{label}</div> }

export default App
