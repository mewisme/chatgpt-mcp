import { useEffect, useState } from "react"
import { Activity, Pencil, RefreshCw, Trash2, Wrench } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { adminApi, type MCPServer, type MCPServerStatus, type MCPServerTools } from "@/lib/api"

type Draft = { server: MCPServer; args: string; env: string; headers: string; tools: string; disabledTools: string }
const emptyServer: MCPServer = { id: "", name: "", transport: "http", enabled: true, url: "", auth: {}, expose: "all", tool_prefix: "", idle_timeout_sec: 600 }
const emptyDraft = (): Draft => ({ server: { ...emptyServer, auth: {} }, args: "", env: "", headers: "", tools: "", disabledTools: "" })

export function ServersPage() {
  const [servers, setServers] = useState<MCPServer[]>([])
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [editingID, setEditingID] = useState("")
  const [status, setStatus] = useState<Record<string, MCPServerStatus>>({})
  const [inspector, setInspector] = useState<MCPServerTools | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  async function load() {
    try { setServers(await adminApi.upstream()); setError("") } catch (value) { setError(errorText(value)) }
  }

  useEffect(() => { void adminApi.upstream().then(setServers).catch((value) => setError(errorText(value))) }, [])

  async function save(event: React.FormEvent) {
    event.preventDefault()
    setBusy(true)
    try {
      const server = buildServer(draft)
      if (editingID) await adminApi.updateUpstream(editingID, server)
      else await adminApi.addUpstream(server)
      cancelEdit()
      await load()
    } catch (value) { setError(errorText(value)) } finally { setBusy(false) }
  }

  async function remove(id: string) {
    setBusy(true)
    try { await adminApi.removeUpstream(id); if (inspector?.server_id === id) setInspector(null); await load() } catch (value) { setError(errorText(value)) } finally { setBusy(false) }
  }

  async function toggle(item: MCPServer) {
    setBusy(true)
    try { await adminApi.updateUpstream(item.id, { ...item, enabled: !item.enabled }); await load() } catch (value) { setError(errorText(value)) } finally { setBusy(false) }
  }

  async function inspect(item: MCPServer) {
    setBusy(true)
    try {
      const [nextStatus, nextTools] = await Promise.all([adminApi.upstreamStatus(item.id, true), adminApi.upstreamTools(item.id, true)])
      setStatus((current) => ({ ...current, [item.id]: nextStatus }))
      setInspector(nextTools)
      setError("")
    } catch (value) { setError(errorText(value)) } finally { setBusy(false) }
  }

  function edit(item: MCPServer) { setEditingID(item.id); setDraft(toDraft(item)); setError(""); window.scrollTo({ top: 0, behavior: "smooth" }) }
  function cancelEdit() { setEditingID(""); setDraft(emptyDraft()) }

  const server = draft.server
  return <div className="space-y-6"><Card><CardHeader><CardTitle>{editingID ? "Edit MCP server" : "Add MCP server"}</CardTitle><CardDescription>Configure HTTP or stdio upstreams. Sensitive header and environment values stay redacted on readback.</CardDescription></CardHeader><CardContent><form className="space-y-5" onSubmit={save}><div className="grid gap-4 md:grid-cols-2"><Field label="ID"><Input required disabled={!!editingID} value={server.id} onChange={(event) => updateServer(setDraft, draft, { id: event.target.value })} /></Field><Field label="Name"><Input required value={server.name} onChange={(event) => updateServer(setDraft, draft, { name: event.target.value })} /></Field><Field label="Transport"><Select value={server.transport} onValueChange={(transport) => updateServer(setDraft, draft, { transport })}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="http">HTTP</SelectItem><SelectItem value="stdio">stdio</SelectItem></SelectContent></Select></Field><Field label="Expose"><Select value={server.expose || "all"} onValueChange={(expose) => updateServer(setDraft, draft, { expose })}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">All tools</SelectItem><SelectItem value="allowlist">Allowlist</SelectItem><SelectItem value="meta_only">Meta only</SelectItem><SelectItem value="none">None</SelectItem></SelectContent></Select></Field><Field label="Tool prefix"><Input placeholder={server.id || "server"} value={server.tool_prefix || ""} onChange={(event) => updateServer(setDraft, draft, { tool_prefix: event.target.value })} /></Field><Field label="Idle timeout (seconds)"><Input type="number" min={0} value={server.idle_timeout_sec || 0} onChange={(event) => updateServer(setDraft, draft, { idle_timeout_sec: Number(event.target.value) })} /></Field></div>{server.transport === "http" ? <HTTPFields draft={draft} setDraft={setDraft} /> : <StdioFields draft={draft} setDraft={setDraft} />}<div className="grid gap-4 md:grid-cols-2"><Field label="Allowlisted tools"><Input placeholder="read_file, search" value={draft.tools} onChange={(event) => setDraft({ ...draft, tools: event.target.value })} /></Field><Field label="Disabled tools"><Input placeholder="delete_file" value={draft.disabledTools} onChange={(event) => setDraft({ ...draft, disabledTools: event.target.value })} /></Field></div><div className="flex items-center justify-between rounded-lg border p-3"><Label>Enabled</Label><Switch checked={server.enabled} onCheckedChange={(enabled) => updateServer(setDraft, draft, { enabled })} /></div>{error ? <div className="text-sm text-destructive">{error}</div> : null}<div className="flex gap-2"><Button disabled={busy} type="submit">{editingID ? "Save server" : "Add server"}</Button>{editingID ? <Button disabled={busy} type="button" variant="outline" onClick={cancelEdit}>Cancel</Button> : null}</div></form></CardContent></Card><Card><CardHeader><CardTitle>MCP servers</CardTitle><CardDescription>Health and tool inspection connect to the upstream immediately.</CardDescription></CardHeader><CardContent className="space-y-3">{servers.length === 0 ? <div className="text-sm text-muted-foreground">No upstream servers configured.</div> : servers.map((item) => <ServerRow key={item.id} item={item} state={status[item.id]} busy={busy} onEdit={() => edit(item)} onInspect={() => inspect(item)} onRemove={() => remove(item.id)} onToggle={() => toggle(item)} />)}</CardContent></Card>{inspector ? <Card><CardHeader><CardTitle>Tools: {inspector.server_id}</CardTitle><CardDescription>{inspector.tools.length} upstream tools, {inspector.proxied_tools.length} currently proxied.</CardDescription></CardHeader><CardContent className="space-y-2">{inspector.tools.map((tool) => <div className="rounded-lg border p-3" key={tool.name}><div className="flex items-center gap-2"><Wrench className="size-4 text-muted-foreground" /><div className="font-mono text-sm">{tool.name}</div>{inspector.proxied_tools.some((name) => name.endsWith(`__${tool.name}`)) ? <Badge variant="secondary">Proxied</Badge> : <Badge variant="outline">Hidden</Badge>}</div>{tool.description ? <div className="mt-1 text-sm text-muted-foreground">{tool.description}</div> : null}</div>)}</CardContent></Card> : null}</div>
}

function HTTPFields({ draft, setDraft }: { draft: Draft; setDraft: React.Dispatch<React.SetStateAction<Draft>> }) {
  const server = draft.server
  return <div className="space-y-4"><div className="grid gap-4 md:grid-cols-2"><Field label="URL"><Input required placeholder="http://127.0.0.1:3000/mcp" value={server.url || ""} onChange={(event) => updateServer(setDraft, draft, { url: event.target.value })} /></Field><Field label="Bearer token env"><Input placeholder="GITHUB_TOKEN" value={server.bearer_token_env_var || ""} onChange={(event) => updateServer(setDraft, draft, { bearer_token_env_var: event.target.value })} /></Field><Field label="Auth mode"><Select value={server.auth?.type || "auto"} onValueChange={(type) => updateServer(setDraft, draft, { auth: { ...server.auth, type } })}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="auto">Auto</SelectItem><SelectItem value="oauth">OAuth</SelectItem><SelectItem value="none">None</SelectItem></SelectContent></Select></Field><Field label="OAuth scope"><Input value={server.auth?.scope || ""} onChange={(event) => updateServer(setDraft, draft, { auth: { ...server.auth, scope: event.target.value } })} /></Field></div><Field label="Headers"><Textarea placeholder={"Authorization=Bearer ...\nX-Header=value"} value={draft.headers} onChange={(event) => setDraft({ ...draft, headers: event.target.value })} /></Field></div>
}

function StdioFields({ draft, setDraft }: { draft: Draft; setDraft: React.Dispatch<React.SetStateAction<Draft>> }) {
  const server = draft.server
  return <div className="space-y-4"><div className="grid gap-4 md:grid-cols-2"><Field label="Command"><Input required placeholder="node" value={server.command || ""} onChange={(event) => updateServer(setDraft, draft, { command: event.target.value })} /></Field><Field label="Working directory"><Input placeholder="/path/to/server" value={server.cwd || ""} onChange={(event) => updateServer(setDraft, draft, { cwd: event.target.value })} /></Field></div><div className="grid gap-4 md:grid-cols-2"><Field label="Arguments, one per line"><Textarea placeholder={"./server.js\n--stdio"} value={draft.args} onChange={(event) => setDraft({ ...draft, args: event.target.value })} /></Field><Field label="Environment"><Textarea placeholder={"NODE_ENV=production\nTOKEN=..."} value={draft.env} onChange={(event) => setDraft({ ...draft, env: event.target.value })} /></Field></div></div>
}

function ServerRow({ item, state, busy, onEdit, onInspect, onRemove, onToggle }: { item: MCPServer; state?: MCPServerStatus; busy: boolean; onEdit: () => void; onInspect: () => void; onRemove: () => void; onToggle: () => void }) {
  return <div className="rounded-lg border p-3"><div className="flex flex-wrap items-center gap-3"><div className="min-w-0 flex-1"><div className="flex items-center gap-2"><div className="truncate font-medium">{item.name}</div><Badge variant={item.enabled ? "secondary" : "outline"}>{item.enabled ? "Enabled" : "Disabled"}</Badge>{state ? <Badge variant={state.health === "connected" ? "secondary" : "outline"}>{state.health}</Badge> : null}</div><div className="mt-1 truncate text-xs text-muted-foreground">{item.id} · {item.transport} · {item.expose || "all"}</div>{state?.last_error ? <div className="mt-1 text-xs text-destructive">{state.last_error}</div> : null}</div><div className="flex items-center gap-1"><Button aria-label="Inspect server" disabled={busy || !item.enabled} size="icon-sm" variant="ghost" onClick={onInspect}><Activity /></Button><Button aria-label="Edit server" disabled={busy} size="icon-sm" variant="ghost" onClick={onEdit}><Pencil /></Button><Button aria-label={item.enabled ? "Disable server" : "Enable server"} disabled={busy} size="icon-sm" variant="ghost" onClick={onToggle}><RefreshCw /></Button><Button aria-label="Remove server" disabled={busy} size="icon-sm" variant="ghost" onClick={onRemove}><Trash2 /></Button></div></div></div>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <div className="space-y-2"><Label>{label}</Label>{children}</div> }
function updateServer(setDraft: React.Dispatch<React.SetStateAction<Draft>>, draft: Draft, patch: Partial<MCPServer>) { setDraft({ ...draft, server: { ...draft.server, ...patch } }) }
function toDraft(server: MCPServer): Draft { return { server: { ...server, auth: { ...server.auth } }, args: (server.args || []).join("\n"), env: formatAssignments(server.env), headers: formatAssignments(server.headers), tools: (server.tools || []).join(", "), disabledTools: (server.disabled_tools || []).join(", ") } }
function buildServer(draft: Draft): MCPServer {
  const server: MCPServer = { ...draft.server, args: lines(draft.args), env: assignments(draft.env), headers: assignments(draft.headers), tools: csv(draft.tools), disabled_tools: csv(draft.disabledTools) }
  if (server.transport === "http") return { ...server, auth: { ...server.auth, type: server.auth?.type || "auto" } }
  return { ...server, auth: { type: "none" } }
}
function assignments(value: string) {
  const result: Record<string, string> = {}
  for (const raw of value.split(/\r?\n/)) {
    if (!raw.trim()) continue
    const index = raw.indexOf("=")
    if (index <= 0) throw new Error(`Expected KEY=VALUE: ${raw}`)
    result[raw.slice(0, index).trim()] = raw.slice(index + 1)
  }
  return result
}
function formatAssignments(value?: Record<string, string>) { return Object.entries(value || {}).map(([key, item]) => `${key}=${item}`).join("\n") }
function lines(value: string) { return value.split(/\r?\n/).map((item) => item.trim()).filter(Boolean) }
function csv(value: string) { return value.split(",").map((item) => item.trim()).filter(Boolean) }
function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
