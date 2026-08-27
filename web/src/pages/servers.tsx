import { useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { adminApi, type MCPServer } from "@/lib/api"

const emptyServer: MCPServer = { id: "", name: "", transport: "http", enabled: true }

export function ServersPage() {
  const [servers, setServers] = useState<MCPServer[]>([])
  const [server, setServer] = useState<MCPServer>(emptyServer)
  const [error, setError] = useState("")

  async function load() { try { setServers(await adminApi.upstream()); setError("") } catch (value) { setError(value instanceof Error ? value.message : String(value)) } }
  useEffect(() => { void adminApi.upstream().then(setServers).catch((value) => setError(value instanceof Error ? value.message : String(value))) }, [])

  async function add(event: React.FormEvent) {
    event.preventDefault()
    try { await adminApi.addUpstream(server); setServer(emptyServer); await load() } catch (value) { setError(value instanceof Error ? value.message : String(value)) }
  }

  async function remove(id: string) { try { await adminApi.removeUpstream(id); await load() } catch (value) { setError(value instanceof Error ? value.message : String(value)) } }

  return <div className="space-y-6"><Card><CardHeader><CardTitle>Add MCP Server</CardTitle></CardHeader><CardContent><form className="grid gap-4 md:grid-cols-2" onSubmit={add}><Field label="ID"><Input required value={server.id} onChange={(event) => setServer({ ...server, id: event.target.value })} /></Field><Field label="Name"><Input required value={server.name} onChange={(event) => setServer({ ...server, name: event.target.value })} /></Field><Field label="Transport"><Input required value={server.transport} onChange={(event) => setServer({ ...server, transport: event.target.value })} /></Field><div className="flex items-end justify-between rounded-lg border p-3"><Label>Enabled</Label><Switch checked={server.enabled} onCheckedChange={(enabled) => setServer({ ...server, enabled })} /></div><Button className="md:w-fit" type="submit">Add server</Button></form></CardContent></Card><Card><CardHeader><CardTitle>MCP Servers</CardTitle></CardHeader><CardContent className="space-y-4">{error ? <div className="text-sm text-destructive">{error}</div> : null}{servers.map((item) => <div className="flex items-center justify-between" key={item.id}><div><div>{item.name}</div><div className="text-sm text-muted-foreground">{item.transport}</div></div><div className="flex items-center gap-2"><Badge>{item.enabled ? "Enabled" : "Disabled"}</Badge><Button size="sm" variant="outline" onClick={() => remove(item.id)}>Remove</Button></div></div>)}</CardContent></Card></div>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <div className="space-y-2"><Label>{label}</Label>{children}</div> }
