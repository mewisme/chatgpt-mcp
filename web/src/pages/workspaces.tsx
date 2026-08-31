import { useEffect, useState } from "react"
import { FolderGit2, Plus, RefreshCw, Trash2 } from "lucide-react"
import { CopyButton } from "@/components/copy-button"
import { DetailRow } from "@/components/detail-row"
import { PageEmpty, PageError, PageLoading } from "@/components/page-state"
import { PageHeader } from "@/components/page-header"
import { ResponsiveDialog } from "@/components/responsive-dialog"
import { TruncatedText } from "@/components/truncated-text"
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Item, ItemActions, ItemContent, ItemDescription, ItemGroup, ItemMedia, ItemTitle } from "@/components/ui/item"
import { Label } from "@/components/ui/label"
import { adminApi, type Workspace } from "@/lib/api"

export function WorkspacesPage() {
  const [items, setItems] = useState<Workspace[]>([])
  const [path, setPath] = useState("")
  const [selected, setSelected] = useState<Workspace | null>(null)
  const [removeTarget, setRemoveTarget] = useState<Workspace | null>(null)
  const [registerOpen, setRegisterOpen] = useState(false)
  const [loading, setLoading] = useState(true)
  const [registering, setRegistering] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState("")

  async function load(manual = false) {
    if (manual) setRefreshing(true)
    try { setItems(await adminApi.workspaces()); setError("") } catch (value) { setError(errorText(value)) } finally { setLoading(false); setRefreshing(false) }
  }

  useEffect(() => { let active = true; void adminApi.workspaces().then((next) => { if (active) { setItems(next); setError("") } }).catch((value) => { if (active) setError(errorText(value)) }).finally(() => { if (active) setLoading(false) }); return () => { active = false } }, [])

  async function register(event: React.FormEvent) {
    event.preventDefault()
    const value = path.trim()
    if (!value) return
    setRegistering(true)
    try { await adminApi.registerWorkspace(value); setPath(""); setRegisterOpen(false); await load() } catch (value) { setError(errorText(value)) } finally { setRegistering(false) }
  }

  async function remove() {
    if (!removeTarget) return
    const target = removeTarget
    setRemoving(true)
    try { await adminApi.removeWorkspace(target.id); if (selected?.id === target.id) setSelected(null); setRemoveTarget(null); await load() } catch (value) { setError(errorText(value)) } finally { setRemoving(false) }
  }

  return <div className="space-y-6"><PageHeader title="Workspaces" description="Canonical project roots available to workspace-bound tools. Unregistering never deletes project files." actions={<><Button disabled={refreshing} size="sm" variant="outline" onClick={() => void load(true)}><RefreshCw className={refreshing ? "animate-spin" : ""} />Refresh</Button><Button size="sm" onClick={() => setRegisterOpen(true)}><Plus />Register workspace</Button></>} /><PageError message={error} />{loading ? <PageLoading rows={4} /> : items.length === 0 ? <PageEmpty icon={FolderGit2} title="No workspaces registered" description="Register a project root to start using workspace-bound tools." action={<Button onClick={() => setRegisterOpen(true)}><Plus />Register workspace</Button>} /> : <ItemGroup>{items.map((item) => <Item className="cursor-pointer" key={item.id} role="button" tabIndex={0} variant="outline" onClick={() => setSelected(item)} onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") setSelected(item) }}><ItemMedia variant="icon"><FolderGit2 className="text-muted-foreground" /></ItemMedia><ItemContent className="min-w-0"><ItemTitle className="w-full"><TruncatedText lines={1}>{item.path}</TruncatedText></ItemTitle><ItemDescription className="font-mono">{item.id}</ItemDescription>{item.allow_dirs?.length ? <div className="mt-1 flex flex-wrap gap-1"><Badge variant="outline">{item.allow_dirs.length} extra path{item.allow_dirs.length === 1 ? "" : "s"}</Badge></div> : null}</ItemContent><ItemActions><Button aria-label={`Unregister ${item.path}`} size="icon-sm" variant="ghost" onClick={(event) => { event.stopPropagation(); setRemoveTarget(item) }}><Trash2 /></Button></ItemActions></Item>)}</ItemGroup>}<ResponsiveDialog open={registerOpen} onOpenChange={setRegisterOpen} title="Register workspace" description="Register one canonical project root. Tools remain locked to its immutable workspace ID." footer={<><Button variant="outline" onClick={() => setRegisterOpen(false)}>Cancel</Button><Button form="register-workspace-form" disabled={registering || !path.trim()} type="submit">{registering ? "Registering..." : "Register workspace"}</Button></>}><form className="space-y-3" id="register-workspace-form" onSubmit={register}><Label htmlFor="workspace-path">Project root</Label><Input autoComplete="off" autoFocus id="workspace-path" placeholder="/home/mew/projects/example" value={path} onChange={(event) => setPath(event.target.value)} /><p className="text-xs text-muted-foreground">Use an absolute path. The server resolves symlinks and validates the canonical root.</p></form></ResponsiveDialog>{selected ? <ResponsiveDialog open onOpenChange={(open) => { if (!open) setSelected(null) }} title="Workspace details" description="Canonical identity and additional filesystem access for this workspace."><div className="divide-y"><DetailRow label="Path" value={<CopyValue value={selected.path} />} /><DetailRow label="Workspace ID" value={<CopyValue value={selected.id} />} /><DetailRow label="Allowed directories" value={selected.allow_dirs?.length ? <div className="space-y-1">{selected.allow_dirs.map((value) => <CopyValue key={value} value={value} />)}</div> : "None"} /></div><div className="mt-4 flex justify-end"><Button variant="destructive" onClick={() => setRemoveTarget(selected)}><Trash2 />Unregister workspace</Button></div></ResponsiveDialog> : null}<AlertDialog open={Boolean(removeTarget)} onOpenChange={(open) => { if (!open && !removing) setRemoveTarget(null) }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Unregister workspace?</AlertDialogTitle><AlertDialogDescription className="break-words">{removeTarget?.path} will be removed from chatgpt-mcp. Project files will not be deleted.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={removing}>Cancel</AlertDialogCancel><AlertDialogAction disabled={removing} variant="destructive" onClick={() => void remove()}>{removing ? "Unregistering..." : "Unregister"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog></div>
}

function CopyValue({ value }: { value: string }) { return <div className="flex min-w-0 items-start gap-1"><span className="min-w-0 flex-1 break-all font-mono text-sm">{value}</span><CopyButton value={value} /></div> }
function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
