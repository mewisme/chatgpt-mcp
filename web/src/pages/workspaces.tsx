import { useEffect, useState } from "react"
import { FolderGit2, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { adminApi, type Workspace } from "@/lib/api"

export function WorkspacesPage() {
  const [items, setItems] = useState<Workspace[]>([])
  const [path, setPath] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  async function load() {
    try { setItems(await adminApi.workspaces()); setError("") } catch (value) { setError(errorText(value)) }
  }

  useEffect(() => { void adminApi.workspaces().then(setItems).catch((value) => setError(errorText(value))) }, [])

  async function register(event: React.FormEvent) {
    event.preventDefault()
    if (!path.trim()) return
    setBusy(true)
    try { await adminApi.registerWorkspace(path.trim()); setPath(""); await load() } catch (value) { setError(errorText(value)) } finally { setBusy(false) }
  }

  async function remove(id: string) {
    setBusy(true)
    try { await adminApi.removeWorkspace(id); await load() } catch (value) { setError(errorText(value)) } finally { setBusy(false) }
  }

  return <div className="space-y-6"><Card><CardHeader><CardTitle>Register workspace</CardTitle><CardDescription>Register one canonical project root. Tools remain locked to its workspace ID.</CardDescription></CardHeader><CardContent><form className="flex flex-col gap-3 sm:flex-row sm:items-end" onSubmit={register}><div className="flex-1 space-y-2"><Label htmlFor="workspace-path">Path</Label><Input id="workspace-path" placeholder="/home/mew/projects/example" value={path} onChange={(event) => setPath(event.target.value)} /></div><Button disabled={busy || !path.trim()} type="submit">Register</Button></form>{error ? <div className="mt-3 text-sm text-destructive">{error}</div> : null}</CardContent></Card><Card><CardHeader><CardTitle>Registered workspaces</CardTitle><CardDescription>Unregistering a workspace removes only its handle. Project files are never deleted.</CardDescription></CardHeader><CardContent className="space-y-3">{items.length === 0 ? <div className="text-sm text-muted-foreground">No workspaces registered.</div> : items.map((item) => <div className="flex items-center gap-3 rounded-lg border p-3" key={item.id}><FolderGit2 className="size-4 shrink-0 text-muted-foreground" /><div className="min-w-0 flex-1"><div className="truncate text-sm font-medium">{item.path}</div><div className="truncate font-mono text-xs text-muted-foreground">{item.id}</div></div><Button aria-label={`Unregister ${item.id}`} disabled={busy} size="icon-sm" variant="ghost" onClick={() => remove(item.id)}><Trash2 /></Button></div>)}</CardContent></Card></div>
}

function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
