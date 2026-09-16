import { useCallback, useEffect, useMemo, useState } from "react"
import { createColumnHelper } from "@tanstack/react-table"
import { Braces, CheckCircle2, MoreHorizontal, Pencil, Plus, RefreshCw, Server as ServerIcon, Trash2, Wrench, XCircle } from "lucide-react"
import { DataTable } from "@/components/data-table"
import { DataTableColumnHeader } from "@/components/data-table-column-header"
import type { DataTableFeatures } from "@/components/data-table-features"
import { DetailRow } from "@/components/detail-row"
import { JsonViewer } from "@/components/json-viewer"
import { PageEmpty, PageError, PageLoading } from "@/components/page-state"
import { PageHeader } from "@/components/page-header"
import { ResponsiveDialog } from "@/components/responsive-dialog"
import { TruncatedText } from "@/components/truncated-text"
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Item, ItemActions, ItemContent, ItemDescription, ItemGroup, ItemHeader, ItemTitle } from "@/components/ui/item"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { ScrollableTabsList, Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { useIsMobile } from "@/hooks/use-mobile"
import { adminApi, type MCPServer, type MCPServerStatus, type MCPServerTools } from "@/lib/api"
import { analyzeMCPServerJSON, formatMCPServerJSON, serverJSONExample, type MCPServerJSONAnalysis } from "@/lib/mcp-server-json"

type Draft = { server: MCPServer; args: string; env: string; headers: string; tools: string; disabledTools: string }
const emptyServer: MCPServer = { id: "", name: "", transport: "http", enabled: true, url: "", expose: "all", tool_prefix: "", idle_timeout_sec: 600 }
const emptyDraft = (): Draft => ({ server: { ...emptyServer }, args: "", env: "", headers: "", tools: "", disabledTools: "" })
const columnHelper = createColumnHelper<DataTableFeatures, MCPServer>()

export function ServersPage() {
  const mobile = useIsMobile()
  const [servers, setServers] = useState<MCPServer[]>([])
  const [selected, setSelected] = useState<MCPServer | null>(null)
  const [removeTarget, setRemoveTarget] = useState<MCPServer | null>(null)
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [editingID, setEditingID] = useState("")
  const [formOpen, setFormOpen] = useState(false)
  const [status, setStatus] = useState<Record<string, MCPServerStatus>>({})
  const [tools, setTools] = useState<Record<string, MCPServerTools>>({})
  const [loading, setLoading] = useState(true)
  const [detailLoading, setDetailLoading] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [busy, setBusy] = useState(false)
  const [busyID, setBusyID] = useState("")
  const [error, setError] = useState("")

  async function load(manual = false) {
    if (manual) setRefreshing(true)
    try {
      const next = await adminApi.upstream()
      setServers(next)
      setError("")
      void loadMetadata(next)
      return next
    } catch (value) { setError(errorText(value)); return [] } finally { setLoading(false); setRefreshing(false) }
  }

  const loadMetadata = useCallback(async (items: MCPServer[]) => {
    const statusResults = await Promise.allSettled(items.map((item) => adminApi.upstreamStatus(item.id, false)))
    const nextStatus: Record<string, MCPServerStatus> = {}
    statusResults.forEach((result, index) => { if (result.status === "fulfilled") nextStatus[items[index].id] = result.value })
    setStatus((current) => ({ ...current, ...nextStatus }))
  }, [])

  useEffect(() => {
    let active = true
    void adminApi.upstream().then((next) => {
      if (!active) return
      setServers(next); setError(""); setLoading(false); void loadMetadata(next)
    }).catch((value) => { if (active) { setError(errorText(value)); setLoading(false) } })
    return () => { active = false }
  }, [loadMetadata])

  async function openDetail(item: MCPServer) {
    setSelected(item)
    setDetailLoading(true)
    try {
      const [nextStatus, nextTools] = await Promise.all([
        adminApi.upstreamStatus(item.id, true),
        adminApi.upstreamTools(item.id, true),
      ])
      setStatus((current) => ({ ...current, [item.id]: nextStatus }))
      setTools((current) => ({ ...current, [item.id]: nextTools }))
      setError("")
    } catch (value) { setError(errorText(value)) } finally { setDetailLoading(false) }
  }

  function addServer() { setEditingID(""); setDraft(emptyDraft()); setFormOpen(true) }
  function editServer(item: MCPServer) { setEditingID(item.id); setDraft(toDraft(item)); setFormOpen(true) }

  async function persistServers(values: MCPServer[]) {
    if (values.length === 0) return
    setBusy(true)
    try {
      if (editingID) await adminApi.updateUpstream(editingID, values[0])
      else for (const value of values) await adminApi.addUpstream(value)
      const next = await load()
      const selectedID = editingID || values[0].id
      if (selected?.id === selectedID) setSelected(next.find((item) => item.id === selectedID) ?? null)
      setFormOpen(false); setEditingID(""); setDraft(emptyDraft())
    } catch (value) { await load(); setError(errorText(value)) } finally { setBusy(false) }
  }

  async function save(event: React.FormEvent) {
    event.preventDefault()
    try {
      const value = buildServer(draft)
      await persistServers([value])
    } catch (value) { setError(errorText(value)) }
  }

  async function toggle(item: MCPServer, enabled: boolean) {
    setBusyID(item.id)
    try {
      await adminApi.updateUpstream(item.id, { ...item, enabled })
      const next = await load()
      const updated = next.find((server) => server.id === item.id)
      if (selected?.id === item.id && updated) setSelected(updated)
    } catch (value) { setError(errorText(value)) } finally { setBusyID("") }
  }

  async function remove() {
    if (!removeTarget) return
    const target = removeTarget
    setBusyID(target.id)
    try { await adminApi.removeUpstream(target.id); if (selected?.id === target.id) setSelected(null); setRemoveTarget(null); await load() } catch (value) { setError(errorText(value)) } finally { setBusyID("") }
  }

  async function refreshDetail() { if (selected) await openDetail(selected) }

  const columns = serverColumns(status, busyID, openDetail, editServer, setRemoveTarget, toggle)
  return <div className="space-y-6"><PageHeader title="MCP Servers" description="Configure HTTP and stdio upstreams and inspect health and tools from the server detail view." actions={<><Button disabled={refreshing} size="sm" variant="outline" onClick={() => void load(true)}><RefreshCw className={refreshing ? "animate-spin" : ""} />Refresh</Button><Button size="sm" onClick={addServer}><Plus />Add MCP server</Button></>} /><PageError message={error} />{loading ? <PageLoading rows={5} /> : servers.length === 0 ? <PageEmpty icon={ServerIcon} title="No MCP servers configured" description="Add an HTTP or stdio upstream to expose its tools through this runtime." action={<Button onClick={addServer}><Plus />Add MCP server</Button>} /> : mobile ? <ServerMobileList servers={servers} status={status} busyID={busyID} onOpen={openDetail} onEdit={editServer} onRemove={setRemoveTarget} onToggle={toggle} /> : <DataTable columns={columns} data={servers} onRowClick={(item) => void openDetail(item)} pageSize={20} />}{formOpen ? <ServerForm key={editingID || "new"} open onOpenChange={setFormOpen} draft={draft} setDraft={setDraft} editingID={editingID} existingIDs={servers.map((item) => item.id)} busy={busy} onSubmit={save} onSaveJSON={(values) => void persistServers(values)} /> : null}{selected ? <ServerDetail server={selected} state={status[selected.id]} tools={tools[selected.id]} loading={detailLoading} onOpenChange={(open) => { if (!open) setSelected(null) }} onRefresh={() => void refreshDetail()} onEdit={() => editServer(selected)} onRemove={() => setRemoveTarget(selected)} /> : null}<AlertDialog open={Boolean(removeTarget)} onOpenChange={(open) => { if (!open && !busyID) setRemoveTarget(null) }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Remove MCP server?</AlertDialogTitle><AlertDialogDescription>{removeTarget?.name} ({removeTarget?.id}) will be removed from the runtime configuration.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={Boolean(busyID)}>Cancel</AlertDialogCancel><AlertDialogAction disabled={Boolean(busyID)} variant="destructive" onClick={() => void remove()}>{busyID ? "Removing..." : "Remove"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog></div>
}

function serverColumns(status: Record<string, MCPServerStatus>, busyID: string, onOpen: (item: MCPServer) => Promise<void>, onEdit: (item: MCPServer) => void, onRemove: (item: MCPServer) => void, onToggle: (item: MCPServer, enabled: boolean) => Promise<void>) {
  return columnHelper.columns([
    columnHelper.accessor("name", { header: ({ column }) => <DataTableColumnHeader column={column} title="Server" />, cell: ({ row }) => <div className="min-w-0"><TruncatedText lines={1} className="font-medium">{row.original.name}</TruncatedText><TruncatedText lines={1} mono className="mt-1 text-xs text-muted-foreground">{row.original.id}</TruncatedText></div> }),
    columnHelper.accessor("transport", { header: "Transport", cell: ({ getValue }) => <Badge variant="outline">{getValue()}</Badge> }),
    columnHelper.display({ id: "health", header: "Health", cell: ({ row }) => <HealthBadge enabled={row.original.enabled} health={status[row.original.id]?.health} /> }),
    columnHelper.accessor("expose", { header: "Expose", cell: ({ getValue }) => <span className="text-xs text-muted-foreground">{getValue() || "all"}</span> }),
    columnHelper.display({ id: "auth", header: "Auth", cell: ({ row }) => <AuthBadge server={row.original} /> }),
    columnHelper.display({ id: "enabled", header: "Enabled", cell: ({ row }) => <div onClick={(event) => event.stopPropagation()}><Switch aria-label={`${row.original.enabled ? "Disable" : "Enable"} ${row.original.name}`} checked={row.original.enabled} disabled={busyID === row.original.id} onCheckedChange={(enabled) => void onToggle(row.original, enabled)} /></div> }),
    columnHelper.display({ id: "actions", header: "", cell: ({ row }) => <div className="flex justify-end" onClick={(event) => event.stopPropagation()}><ServerActions item={row.original} onOpen={() => void onOpen(row.original)} onEdit={() => onEdit(row.original)} onRemove={() => onRemove(row.original)} /></div> }),
  ])
}

function ServerMobileList({ servers, status, busyID, onOpen, onEdit, onRemove, onToggle }: { servers: MCPServer[]; status: Record<string, MCPServerStatus>; busyID: string; onOpen: (item: MCPServer) => Promise<void>; onEdit: (item: MCPServer) => void; onRemove: (item: MCPServer) => void; onToggle: (item: MCPServer, enabled: boolean) => Promise<void> }) {
  return <ItemGroup>{servers.map((item) => <Item className="cursor-pointer" key={item.id} role="button" tabIndex={0} variant="outline" onClick={() => void onOpen(item)} onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") void onOpen(item) }}><ItemContent className="min-w-0"><ItemHeader><ItemTitle>{item.name}</ItemTitle><HealthBadge enabled={item.enabled} health={status[item.id]?.health} /></ItemHeader><ItemDescription>{item.id} · {item.transport} · {item.expose || "all"}</ItemDescription><div className="flex flex-wrap gap-1"><AuthBadge server={item} /></div></ItemContent><ItemActions onClick={(event) => event.stopPropagation()}><Switch aria-label={`${item.enabled ? "Disable" : "Enable"} ${item.name}`} checked={item.enabled} disabled={busyID === item.id} onCheckedChange={(enabled) => void onToggle(item, enabled)} /><ServerActions item={item} onOpen={() => void onOpen(item)} onEdit={() => onEdit(item)} onRemove={() => onRemove(item)} /></ItemActions></Item>)}</ItemGroup>
}

function ServerActions({ item, onOpen, onEdit, onRemove }: { item: MCPServer; onOpen: () => void; onEdit: () => void; onRemove: () => void }) {
  return <DropdownMenu><DropdownMenuTrigger asChild><Button aria-label={`Actions for ${item.name}`} size="icon-sm" variant="ghost"><MoreHorizontal /></Button></DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuItem onClick={onOpen}><ServerIcon />View details</DropdownMenuItem><DropdownMenuItem onClick={onEdit}><Pencil />Edit</DropdownMenuItem><DropdownMenuSeparator /><DropdownMenuItem variant="destructive" onClick={onRemove}><Trash2 />Remove</DropdownMenuItem></DropdownMenuContent></DropdownMenu>
}

function ServerDetail({ server, state, tools, loading, onOpenChange, onRefresh, onEdit, onRemove }: { server: MCPServer; state?: MCPServerStatus; tools?: MCPServerTools; loading: boolean; onOpenChange: (open: boolean) => void; onRefresh: () => void; onEdit: () => void; onRemove: () => void }) {
  return <ResponsiveDialog open onOpenChange={onOpenChange} title={server.name} description={server.id} className="pb-1"><div className="mb-4 flex flex-wrap items-center justify-between gap-2"><div className="flex flex-wrap gap-1"><HealthBadge enabled={server.enabled} health={state?.health} /><AuthBadge server={server} /><Badge variant="outline">{server.transport}</Badge></div><div className="flex gap-1"><Button aria-label="Refresh server details" disabled={loading} size="icon-sm" variant="ghost" onClick={onRefresh}><RefreshCw className={loading ? "animate-spin" : ""} /></Button><Button size="sm" variant="outline" onClick={onEdit}><Pencil />Edit</Button></div></div>{loading && !state ? <PageLoading rows={4} /> : <Tabs defaultValue="overview"><ScrollableTabsList><TabsTrigger value="overview">Overview</TabsTrigger><TabsTrigger value="tools">Tools</TabsTrigger><TabsTrigger value="config">Configuration</TabsTrigger></ScrollableTabsList><TabsContent className="mt-4" value="overview"><div className="divide-y"><DetailRow label="Health" value={<HealthBadge enabled={server.enabled} health={state?.health} />} /><DetailRow label="Enabled" value={server.enabled ? "Yes" : "No"} /><DetailRow label="Transport" value={server.transport} mono /><DetailRow label="Expose" value={server.expose || "all"} mono /><DetailRow label="Auth" value={state?.auth || authLabel(server)} mono /><DetailRow label={server.transport === "http" ? "URL" : "Command"} value={server.transport === "http" ? server.url || "-" : [server.command, ...(server.args || [])].filter(Boolean).join(" ") || "-"} mono /><DetailRow label="Tool count" value={state?.tool_count ?? tools?.tools.length ?? "-"} /><DetailRow label="PID" value={state?.pid ?? "-"} mono /></div>{state?.last_error ? <div className="mt-4 break-words rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{state.last_error}</div> : null}</TabsContent><TabsContent className="mt-4" value="tools">{!tools ? <PageLoading rows={4} /> : tools.tools.length === 0 ? <PageEmpty icon={Wrench} title="No upstream tools" description="This server did not report any tools." /> : <ItemGroup>{tools.tools.map((tool) => { const proxied = tools.proxied_tools.some((name) => name === tool.name || name.endsWith(`__${tool.name}`)); return <Item key={tool.name} variant="outline"><ItemContent className="min-w-0"><ItemHeader><ItemTitle className="font-mono">{tool.name}</ItemTitle><Badge variant={proxied ? "secondary" : "outline"}>{proxied ? "Proxied" : "Hidden"}</Badge></ItemHeader><ItemDescription>{tool.description || "No description."}</ItemDescription></ItemContent></Item> })}</ItemGroup>}</TabsContent><TabsContent className="mt-4" value="config"><JsonViewer value={server} /></TabsContent></Tabs>}<div className="mt-5 flex justify-end"><Button variant="destructive" onClick={onRemove}><Trash2 />Remove server</Button></div></ResponsiveDialog>
}

function ServerForm({ open, onOpenChange, draft, setDraft, editingID, existingIDs, busy, onSubmit, onSaveJSON }: { open: boolean; onOpenChange: (open: boolean) => void; draft: Draft; setDraft: React.Dispatch<React.SetStateAction<Draft>>; editingID: string; existingIDs: string[]; busy: boolean; onSubmit: (event: React.FormEvent) => void; onSaveJSON: (servers: MCPServer[]) => void }) {
  const editing = Boolean(editingID)
  const server = draft.server
  const [mode, setMode] = useState<"form" | "json">("form")
  const [jsonText, setJsonText] = useState(() => editing ? JSON.stringify(buildServer(draft), null, 2) : "")
  const analysis = useMemo(() => reviewJSONAnalysis(analyzeMCPServerJSON(jsonText), existingIDs, editingID), [jsonText, existingIDs, editingID])
  const validServers = analysis.items.flatMap((item) => item.server && item.errors.length === 0 ? [item.server] : [])
  const canSaveJSON = !analysis.error && validServers.length > 0 && (!editing || (analysis.kind === "single" && analysis.items.length === 1 && analysis.items[0].errors.length === 0))

  function selectMode(next: string) {
    const value = next === "json" ? "json" : "form"
    if (value === "json" && mode === "form" && (editing || server.id || server.name || server.command || server.url)) {
      try { setJsonText(JSON.stringify(buildServer(draft), null, 2)) } catch { /* keep the current JSON editor content */ }
    }
    setMode(value)
  }

  function useInForm() {
    if (validServers.length !== 1) return
    setDraft(toDraft(validServers[0])); setMode("form")
  }

  const footer = mode === "form" ? <><Button disabled={busy} variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button><Button disabled={busy} form="mcp-server-form" type="submit">{busy ? "Saving..." : editing ? "Save server" : "Add server"}</Button></> : <><Button disabled={busy} variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button><Button disabled={busy || !canSaveJSON} onClick={() => onSaveJSON(validServers)}>{busy ? "Saving..." : editing ? "Save JSON" : `Import ${validServers.length} server${validServers.length === 1 ? "" : "s"}`}</Button></>
  return <ResponsiveDialog open={open} onOpenChange={onOpenChange} wide title={editing ? "Edit MCP server" : "Add MCP server"} description={editing ? "Edit with the structured form or the raw server JSON. Sensitive values stay redacted on readback." : "Add one server with the form, paste raw server JSON, or import an mcpServers config map."} footer={footer}><Tabs value={mode} onValueChange={selectMode}><TabsList className="mb-5"><TabsTrigger value="form">Form</TabsTrigger><TabsTrigger value="json"><Braces />{editing ? "JSON" : "JSON import"}</TabsTrigger></TabsList><TabsContent value="form"><form className="space-y-5" id="mcp-server-form" onSubmit={onSubmit}><div className="grid gap-4 md:grid-cols-2"><Field label="ID"><Input required disabled={editing} value={server.id} onChange={(event) => updateServer(setDraft, draft, { id: event.target.value })} /></Field><Field label="Name"><Input required value={server.name} onChange={(event) => updateServer(setDraft, draft, { name: event.target.value })} /></Field><Field label="Transport"><Select value={server.transport} onValueChange={(transport) => updateServer(setDraft, draft, { transport })}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="http">HTTP</SelectItem><SelectItem value="stdio">stdio</SelectItem></SelectContent></Select></Field><Field label="Expose"><Select value={server.expose || "all"} onValueChange={(expose) => updateServer(setDraft, draft, { expose })}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">All tools</SelectItem><SelectItem value="allowlist">Allowlist</SelectItem><SelectItem value="meta_only">Meta only</SelectItem><SelectItem value="none">None</SelectItem></SelectContent></Select></Field><Field label="Tool prefix"><Input placeholder={server.id || "server"} value={server.tool_prefix || ""} onChange={(event) => updateServer(setDraft, draft, { tool_prefix: event.target.value })} /></Field><Field label="Idle timeout (seconds)"><Input min={0} type="number" value={server.idle_timeout_sec || 0} onChange={(event) => updateServer(setDraft, draft, { idle_timeout_sec: Number(event.target.value) })} /></Field></div>{server.transport === "http" ? <HTTPFields draft={draft} setDraft={setDraft} /> : <StdioFields draft={draft} setDraft={setDraft} />}<div className="grid gap-4 md:grid-cols-2"><Field label="Allowlisted tools"><Input placeholder="read_file, search" value={draft.tools} onChange={(event) => setDraft({ ...draft, tools: event.target.value })} /></Field><Field label="Disabled tools"><Input placeholder="delete_file" value={draft.disabledTools} onChange={(event) => setDraft({ ...draft, disabledTools: event.target.value })} /></Field></div><label className="flex items-center justify-between rounded-lg border p-3"><span className="text-sm font-medium">Enabled</span><Switch checked={server.enabled} onCheckedChange={(enabled) => updateServer(setDraft, draft, { enabled })} /></label></form></TabsContent><TabsContent className="space-y-4" value="json"><div className="flex flex-wrap items-center justify-between gap-2"><div className="text-sm text-muted-foreground">{editing ? "Edit the complete server object. The ID cannot be changed." : "Paste a server object, an array, or a config containing mcpServers."}</div><div className="flex flex-wrap gap-1"><Button disabled={!jsonText.trim()} size="sm" variant="outline" onClick={() => { try { setJsonText(formatMCPServerJSON(jsonText)) } catch { /* validation below shows the parse error */ } }}>Format</Button>{!editing ? <Button size="sm" variant="outline" onClick={() => setJsonText(serverJSONExample())}>Use example</Button> : null}<Button disabled={!jsonText} size="sm" variant="ghost" onClick={() => setJsonText("")}>Clear</Button></div></div><Textarea aria-label="MCP server JSON" autoCapitalize="off" autoCorrect="off" className="min-h-72 resize-y font-mono text-xs leading-relaxed md:min-h-96" placeholder={editing ? "{\n  \"id\": \"server\",\n  ...\n}" : "{\n  \"mcpServers\": {\n    \"server\": { ... }\n  }\n}"} spellCheck={false} value={jsonText} onChange={(event) => setJsonText(event.target.value)} /><JSONImportPreview analysis={analysis} validCount={validServers.length} editing={editing} onUseInForm={useInForm} /></TabsContent></Tabs></ResponsiveDialog>
}

function JSONImportPreview({ analysis, validCount, editing, onUseInForm }: { analysis: MCPServerJSONAnalysis; validCount: number; editing: boolean; onUseInForm: () => void }) {
  if (analysis.error) return <Alert variant="destructive"><XCircle /><AlertDescription>{analysis.error}</AlertDescription></Alert>
  if (analysis.items.length === 0) return <Alert><Braces /><AlertDescription>No server entries detected.</AlertDescription></Alert>
  return <div className="space-y-3"><div className="flex flex-wrap items-center justify-between gap-2"><div className="text-sm font-medium">Detected {analysis.items.length} server{analysis.items.length === 1 ? "" : "s"}</div><div className="flex items-center gap-2"><Badge variant={validCount === analysis.items.length ? "secondary" : "outline"}>{validCount} valid</Badge>{validCount === 1 ? <Button size="sm" variant="outline" onClick={onUseInForm}>Use in form</Button> : null}</div></div><div className="space-y-2">{analysis.items.map((item, index) => <div className="rounded-lg border p-3" key={`${item.key}-${index}`}><div className="flex items-start gap-3"><div className={item.errors.length === 0 ? "text-emerald-600 dark:text-emerald-400" : "text-destructive"}>{item.errors.length === 0 ? <CheckCircle2 className="mt-0.5 size-4" /> : <XCircle className="mt-0.5 size-4" />}</div><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><span className="break-all font-mono text-sm font-medium">{item.server?.id || item.key}</span>{item.server ? <><Badge variant="outline">{item.server.transport}</Badge><Badge variant="outline">{item.server.enabled ? "Enabled" : "Disabled"}</Badge></> : null}</div>{item.server?.name && item.server.name !== item.server.id ? <div className="mt-1 text-sm text-muted-foreground">{item.server.name}</div> : null}{item.errors.length > 0 ? <ul className="mt-2 list-disc space-y-1 pl-4 text-xs text-destructive">{item.errors.map((error) => <li key={error}>{error}</li>)}</ul> : null}</div></div></div>)}</div>{!editing && validCount > 0 && validCount < analysis.items.length ? <Alert><AlertDescription>Only valid entries will be imported. Fix invalid entries if you want to import the entire config.</AlertDescription></Alert> : null}</div>
}

function reviewJSONAnalysis(analysis: MCPServerJSONAnalysis, existingIDs: string[], editingID: string): MCPServerJSONAnalysis {
  if (analysis.error) return analysis
  if (editingID && analysis.kind !== "single") return { ...analysis, error: "Editing accepts one raw server object, not a collection." }
  const existing = new Set(existingIDs)
  return { ...analysis, items: analysis.items.map((item) => {
    if (!item.server) return item
    const errors = [...item.errors]
    if (editingID && item.server.id !== editingID) errors.push(`id cannot be changed from ${editingID}.`)
    if (!editingID && existing.has(item.server.id)) errors.push(`server ${item.server.id} already exists.`)
    return { ...item, errors }
  }) }
}

function HTTPFields({ draft, setDraft }: { draft: Draft; setDraft: React.Dispatch<React.SetStateAction<Draft>> }) {
  const server = draft.server
  return <div className="space-y-4"><div className="grid gap-4 md:grid-cols-2"><Field label="URL"><Input required placeholder="http://127.0.0.1:3000/mcp" value={server.url || ""} onChange={(event) => updateServer(setDraft, draft, { url: event.target.value })} /></Field><Field label="Bearer token env"><Input placeholder="GITHUB_TOKEN" value={server.bearer_token_env_var || ""} onChange={(event) => updateServer(setDraft, draft, { bearer_token_env_var: event.target.value })} /></Field></div><Field label="Headers"><Textarea placeholder={"Authorization=Bearer ...\nX-Header=value"} value={draft.headers} onChange={(event) => setDraft({ ...draft, headers: event.target.value })} /></Field></div>
}

function StdioFields({ draft, setDraft }: { draft: Draft; setDraft: React.Dispatch<React.SetStateAction<Draft>> }) {
  const server = draft.server
  return <div className="space-y-4"><div className="grid gap-4 md:grid-cols-2"><Field label="Command"><Input required placeholder="node" value={server.command || ""} onChange={(event) => updateServer(setDraft, draft, { command: event.target.value })} /></Field><Field label="Working directory"><Input placeholder="/path/to/server" value={server.cwd || ""} onChange={(event) => updateServer(setDraft, draft, { cwd: event.target.value })} /></Field></div><div className="grid gap-4 md:grid-cols-2"><Field label="Arguments, one per line"><Textarea placeholder={"./server.js\n--stdio"} value={draft.args} onChange={(event) => setDraft({ ...draft, args: event.target.value })} /></Field><Field label="Environment"><Textarea placeholder={"NODE_ENV=production\nTOKEN=..."} value={draft.env} onChange={(event) => setDraft({ ...draft, env: event.target.value })} /></Field></div></div>
}

function HealthBadge({ enabled, health }: { enabled: boolean; health?: string }) {
  if (!enabled) return <Badge variant="outline">Disabled</Badge>
  if (!health || health === "unknown") return <Badge variant="outline">Unknown</Badge>
  return <Badge variant={health === "connected" ? "secondary" : health === "unreachable" ? "destructive" : "outline"}>{health}</Badge>
}

function AuthBadge({ server }: { server: MCPServer }) {
  return <Badge variant="outline">{authLabel(server)}</Badge>
}

function authLabel(server: MCPServer) {
  if (server.bearer_token_env_var) return "Bearer"
  if (Object.keys(server.headers || {}).some((key) => key.toLowerCase() === "authorization")) return "Bearer"
  return "None"
}
function Field({ label, children }: { label: string; children: React.ReactNode }) { return <div className="space-y-2"><Label>{label}</Label>{children}</div> }
function updateServer(setDraft: React.Dispatch<React.SetStateAction<Draft>>, draft: Draft, patch: Partial<MCPServer>) { setDraft({ ...draft, server: { ...draft.server, ...patch } }) }
function toDraft(server: MCPServer): Draft { return { server: { ...server }, args: (server.args || []).join("\n"), env: formatAssignments(server.env), headers: formatAssignments(server.headers), tools: (server.tools || []).join(", "), disabledTools: (server.disabled_tools || []).join(", ") } }
function buildServer(draft: Draft): MCPServer { return { ...draft.server, args: lines(draft.args), env: assignments(draft.env), headers: assignments(draft.headers), tools: csv(draft.tools), disabled_tools: csv(draft.disabledTools) } }
function assignments(value: string) { const result: Record<string, string> = {}; for (const raw of value.split(/\r?\n/)) { if (!raw.trim()) continue; const index = raw.indexOf("="); if (index <= 0) throw new Error(`Expected KEY=VALUE: ${raw}`); result[raw.slice(0, index).trim()] = raw.slice(index + 1) }; return result }
function formatAssignments(value?: Record<string, string>) { return Object.entries(value || {}).map(([key, item]) => `${key}=${item}`).join("\n") }
function lines(value: string) { return value.split(/\r?\n/).map((item) => item.trim()).filter(Boolean) }
function csv(value: string) { return value.split(",").map((item) => item.trim()).filter(Boolean) }
function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
