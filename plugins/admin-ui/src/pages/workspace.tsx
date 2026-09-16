import { Suspense, useEffect, useState } from "react"
import { ArrowLeft, RefreshCw } from "lucide-react"
import { Link, NavLink, Outlet, useParams } from "react-router-dom"
import { PageError, PageLoading } from "@/components/page-state"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import { adminApi, type Workspace } from "@/lib/api"
import { cn } from "@/lib/utils"

export type WorkspaceOutletContext = { workspace: Workspace }

export function WorkspaceLayout() {
  const { workspaceID = "" } = useParams<{ workspaceID: string }>()
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState("")

  async function load(refresh = false) {
    if (!workspaceID) return
    if (refresh) setRefreshing(true)
    try { setWorkspace(await adminApi.workspace(workspaceID)); setError("") } catch (value) { setError(errorText(value)) } finally { setLoading(false); setRefreshing(false) }
  }

  useEffect(() => { let active = true; if (!workspaceID) return; void adminApi.workspace(workspaceID).then((value) => { if (active) { setWorkspace(value); setError("") } }).catch((value) => { if (active) setError(errorText(value)) }).finally(() => { if (active) setLoading(false) }); return () => { active = false } }, [workspaceID])

  return <div className="space-y-6"><PageHeader title="Workspace" description={workspace?.path || workspaceID} actions={<><Button asChild size="sm" variant="outline"><Link to="/workspaces"><ArrowLeft />Workspaces</Link></Button><Button disabled={refreshing} size="sm" variant="outline" onClick={() => void load(true)}><RefreshCw className={refreshing ? "animate-spin" : ""} />Refresh</Button></>} /><PageError message={error} />{loading ? <PageLoading rows={4} /> : workspace ? <><WorkspaceNav workspaceID={workspace.id} /><Suspense fallback={<PageLoading rows={4} />}><Outlet context={{ workspace } satisfies WorkspaceOutletContext} /></Suspense></> : null}</div>
}

function WorkspaceNav({ workspaceID }: { workspaceID: string }) {
  const base = `/workspaces/${encodeURIComponent(workspaceID)}`
  return <ScrollArea className="w-full" scrollbars="horizontal"><div aria-label="Workspace sections" className="inline-flex h-8 w-max min-w-full items-center rounded-lg bg-muted p-[3px] text-muted-foreground" role="tablist"><WorkspaceNavLink end label="Overview" to={base} /><WorkspaceNavLink label="Context" to={`${base}/context`} /><WorkspaceNavLink label="Requests" to={`${base}/requests`} /><WorkspaceNavLink label="Activity" to={`${base}/activity`} /><WorkspaceNavLink label="Plugins" to={`${base}/plugins`} /></div></ScrollArea>
}

function WorkspaceNavLink({ to, label, end = false }: { to: string; label: string; end?: boolean }) {
  return <NavLink end={end} role="tab" to={to} className={({ isActive }) => cn("inline-flex h-[calc(100%-1px)] flex-1 items-center justify-center rounded-md px-2 py-0.5 text-sm font-medium whitespace-nowrap text-foreground/60 transition-all hover:text-foreground focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50", isActive && "bg-background text-foreground shadow-sm dark:bg-input/30")}>{label}</NavLink>
}

function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
