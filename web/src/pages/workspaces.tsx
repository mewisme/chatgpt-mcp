import { useEffect, useState } from "react"
import { FolderGit2, RefreshCw, Trash2 } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { adminApi, type Workspace } from "@/lib/api"

export function WorkspacesPage() {
  const [items, setItems] = useState<Workspace[]>([])
  const [path, setPath] = useState("")
  const [loading, setLoading] = useState(true)
  const [registering, setRegistering] = useState(false)
  const [removing, setRemoving] = useState("")
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState("")

  async function load(manual = false) {
    if (manual) setRefreshing(true)
    try { setItems(await adminApi.workspaces()); setError("") } catch (value) { setError(errorText(value)) } finally { setLoading(false); setRefreshing(false) }
  }

  useEffect(() => {
    let active = true
    void adminApi.workspaces().then((next) => { if (active) { setItems(next); setError("") } }).catch((value) => { if (active) setError(errorText(value)) }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  async function register(event: React.FormEvent) {
    event.preventDefault()
    const value = path.trim()
    if (!value) return
    setRegistering(true)
    try { await adminApi.registerWorkspace(value); setPath(""); await load() } catch (value) { setError(errorText(value)) } finally { setRegistering(false) }
  }

  async function remove(item: Workspace) {
    if (!window.confirm(`Unregister workspace?\n\n${item.path}\n\nProject files will not be deleted.`)) return
    setRemoving(item.id)
    try { await adminApi.removeWorkspace(item.id); await load() } catch (value) { setError(errorText(value)) } finally { setRemoving("") }
  }

  return <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]"><Card className="h-fit"><CardHeader><CardTitle>Register workspace</CardTitle><CardDescription>Register one canonical project root. Tools remain locked to its immutable workspace ID.</CardDescription></CardHeader><CardContent><form className="space-y-4" onSubmit={register}><div className="space-y-2"><Label htmlFor="workspace-path">Project root</Label><Input autoComplete="off" id="workspace-path" placeholder="/home/mew/projects/example" value={path} onChange={(event) => setPath(event.target.value)} /><div className="text-xs text-muted-foreground">Use an absolute path. The server resolves and validates the canonical root.</div></div><Button className="w-full sm:w-auto" disabled={registering || !path.trim()} type="submit">{registering ? "Registering..." : "Register workspace"}</Button></form>{error ? <div className="mt-4 rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">{error}</div> : null}</CardContent></Card><Card><CardHeader className="gap-3 sm:flex-row sm:items-center sm:justify-between"><div><CardTitle className="flex items-center gap-2">Registered workspaces <Badge variant="secondary">{items.length}</Badge></CardTitle><CardDescription>Unregistering removes only the handle. Project files are never deleted.</CardDescription></div><Button disabled={refreshing} size="sm" variant="outline" onClick={() => void load(true)}><RefreshCw className={refreshing ? "animate-spin" : ""} />Refresh</Button></CardHeader><CardContent className="space-y-3">{loading ? <div className="py-10 text-center text-sm text-muted-foreground">Loading workspaces...</div> : items.length === 0 ? <div className="rounded-lg border border-dashed py-10 text-center"><FolderGit2 className="mx-auto size-6 text-muted-foreground" /><div className="mt-3 text-sm font-medium">No workspaces registered</div><div className="mt-1 text-xs text-muted-foreground">Register a project root to start using workspace-bound tools.</div></div> : items.map((item) => <div className="flex items-center gap-3 rounded-lg border p-3 transition-colors hover:bg-muted/40" key={item.id}><FolderGit2 className="size-4 shrink-0 text-muted-foreground" /><div className="min-w-0 flex-1"><div className="truncate text-sm font-medium" title={item.path}>{item.path}</div><div className="truncate font-mono text-xs text-muted-foreground" title={item.id}>{item.id}</div></div><Button aria-label={`Unregister ${item.path}`} disabled={removing === item.id} size="icon-sm" variant="ghost" onClick={() => void remove(item)}><Trash2 /></Button></div>)}</CardContent></Card></div>
}

function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
