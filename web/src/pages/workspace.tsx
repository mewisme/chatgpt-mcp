import { useEffect, useState } from "react"
import { ArrowLeft, FolderGit2, RefreshCw } from "lucide-react"
import { CopyButton } from "@/components/copy-button"
import { DetailRow } from "@/components/detail-row"
import { PageError, PageLoading } from "@/components/page-state"
import { PageHeader } from "@/components/page-header"
import { WorkspaceContext } from "@/components/workspace-context"
import { Button } from "@/components/ui/button"
import { ScrollableTabsList, Tabs, TabsContent, TabsTrigger } from "@/components/ui/tabs"
import { useAdminRouter } from "@/lib/use-admin-router"
import { adminApi, type Workspace } from "@/lib/api"

export function WorkspacePage() {
  const { route, navigate } = useAdminRouter()
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState("")
  const workspaceID = route.workspaceID ?? ""

  async function load(refresh = false) {
    if (!workspaceID) return
    if (refresh) setRefreshing(true)
    try { setWorkspace(await adminApi.workspace(workspaceID)); setError("") } catch (value) { setError(errorText(value)) } finally { setLoading(false); setRefreshing(false) }
  }

  useEffect(() => { let active = true; if (!workspaceID) return; void adminApi.workspace(workspaceID).then((value) => { if (active) { setWorkspace(value); setError("") } }).catch((value) => { if (active) setError(errorText(value)) }).finally(() => { if (active) setLoading(false) }); return () => { active = false } }, [workspaceID])

  const active = route.id === "workspace-context" ? "context" : route.id === "workspace-requests" ? "requests" : route.id === "workspace-activity" ? "activity" : "overview"
  function navigateTab(value: string) { const base = `/workspaces/${encodeURIComponent(workspaceID)}`; navigate(value === "overview" ? base : `${base}/${value}`) }

  return <div className="space-y-6"><PageHeader title="Workspace" description={workspace?.path || workspaceID} actions={<><Button size="sm" variant="outline" onClick={() => navigate("/workspaces")}><ArrowLeft />Workspaces</Button><Button disabled={refreshing} size="sm" variant="outline" onClick={() => void load(true)}><RefreshCw className={refreshing ? "animate-spin" : ""} />Refresh</Button></>} /><PageError message={error} />{loading ? <PageLoading rows={4} /> : workspace ? <Tabs value={active} onValueChange={navigateTab}><ScrollableTabsList className="justify-start"><TabsTrigger value="overview">Overview</TabsTrigger><TabsTrigger value="context">Context</TabsTrigger><TabsTrigger value="requests">Requests</TabsTrigger><TabsTrigger value="activity">Activity</TabsTrigger></ScrollableTabsList><TabsContent className="mt-6" value="overview"><div className="rounded-xl border bg-card"><div className="flex items-center gap-3 border-b p-4"><div className="flex size-9 items-center justify-center rounded-lg bg-muted"><FolderGit2 className="size-4 text-muted-foreground" /></div><div className="min-w-0"><div className="truncate text-sm font-medium">{workspace.path}</div><div className="font-mono text-xs text-muted-foreground">{workspace.id}</div></div></div><div className="divide-y px-4"><DetailRow label="Path" value={<CopyValue value={workspace.path} />} /><DetailRow label="Workspace ID" value={<CopyValue value={workspace.id} />} /><DetailRow label="Allowed directories" value={workspace.allow_dirs?.length ? <div className="space-y-1">{workspace.allow_dirs.map((value) => <CopyValue key={value} value={value} />)}</div> : "None"} /></div></div></TabsContent><TabsContent className="mt-6" value="context"><WorkspaceContext workspaceID={workspace.id} /></TabsContent><TabsContent className="mt-6" value="requests"><div className="rounded-xl border bg-card p-6 text-sm text-muted-foreground">Workspace-scoped requests will be shown here in the next phase.</div></TabsContent><TabsContent className="mt-6" value="activity"><div className="rounded-xl border bg-card p-6 text-sm text-muted-foreground">Workspace-scoped activity and command streams will be shown here after execution streaming is enabled.</div></TabsContent></Tabs> : null}</div>
}

function CopyValue({ value }: { value: string }) { return <div className="flex min-w-0 items-start gap-1"><span className="min-w-0 flex-1 break-all font-mono text-sm">{value}</span><CopyButton value={value} /></div> }
function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }