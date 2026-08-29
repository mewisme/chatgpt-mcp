import { useEffect, useMemo, useState } from "react"
import { CircleDot, Trash2 } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { authHeaders } from "@/lib/api"

type ActivityEvent = {
  kind: string
  method?: string
  source?: string
  tool?: string
  workspace_id?: string
  status?: string
  duration_ms?: number
  message?: string
  timestamp: string
}

export function ActivityPage() {
  const [events, setEvents] = useState<ActivityEvent[]>([])
  const [kind, setKind] = useState("all")
  const [status, setStatus] = useState("all")
  const [query, setQuery] = useState("")
  const [connected, setConnected] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    const controller = new AbortController()
    void streamActivity(controller.signal, (event) => {
      setConnected(true)
      setEvents((items) => [event, ...items].slice(0, 200))
    }).catch((value) => {
      if (!controller.signal.aborted) {
        setConnected(false)
        setError(errorText(value))
      }
    })
    return () => controller.abort()
  }, [])

  const kinds = useMemo(() => [...new Set(events.map((event) => event.kind))].sort(), [events])
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return events.filter((event) => {
      if (kind !== "all" && event.kind !== kind) return false
      if (status !== "all" && event.status !== status) return false
      if (!needle) return true
      return [event.kind, event.method, event.source, event.tool, event.workspace_id, event.status, event.message].some((value) => value?.toLowerCase().includes(needle))
    })
  }, [events, kind, status, query])

  return <div className="space-y-6"><Card><CardHeader><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle>Activity</CardTitle><CardDescription>Live MCP requests and tool calls. The stream replays recent in-memory events when this page opens.</CardDescription></div><Badge variant={connected ? "secondary" : "outline"}><CircleDot className="mr-1 size-3" />{connected ? "Live" : "Connecting"}</Badge></div></CardHeader><CardContent className="space-y-4"><div className="flex flex-col gap-2 lg:flex-row"><Input className="lg:flex-1" placeholder="Filter tool, workspace, message..." value={query} onChange={(event) => setQuery(event.target.value)} /><Select value={kind} onValueChange={setKind}><SelectTrigger className="w-full lg:w-48"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">All event types</SelectItem>{kinds.map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select><Select value={status} onValueChange={setStatus}><SelectTrigger className="w-full lg:w-40"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">All statuses</SelectItem><SelectItem value="ok">OK</SelectItem><SelectItem value="error">Error</SelectItem></SelectContent></Select><Button variant="outline" onClick={() => setEvents([])}><Trash2 />Clear</Button></div>{error ? <div className="text-sm text-destructive">{error}</div> : null}<div className="space-y-2">{filtered.length === 0 ? <div className="text-sm text-muted-foreground">No matching activity.</div> : filtered.map((event, index) => <ActivityRow event={event} key={`${event.timestamp}-${event.kind}-${event.tool || event.method || ""}-${index}`} />)}</div></CardContent></Card></div>
}

function ActivityRow({ event }: { event: ActivityEvent }) {
  const title = event.tool || event.method || event.kind
  return <div className="rounded-lg border p-3"><div className="flex flex-wrap items-center gap-2"><div className="font-mono text-sm font-medium">{title}</div><Badge variant="outline">{event.kind}</Badge>{event.status ? <Badge variant={event.status === "error" ? "destructive" : "secondary"}>{event.status}</Badge> : null}<div className="ml-auto text-xs text-muted-foreground">{formatTime(event.timestamp)}</div></div><div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">{event.source ? <span>source {event.source}</span> : null}{event.workspace_id ? <span>workspace {event.workspace_id}</span> : null}{event.duration_ms !== undefined ? <span>{event.duration_ms} ms</span> : null}</div>{event.message ? <div className="mt-2 break-words text-sm text-muted-foreground">{event.message}</div> : null}</div>
}

async function streamActivity(signal: AbortSignal, onEvent: (event: ActivityEvent) => void) {
  const response = await fetch("/api/activity/stream?history=100", { headers: authHeaders(), signal })
  if (!response.ok || !response.body) {
    const message = await response.text().catch(() => "")
    throw new Error(message.trim() || `Activity stream ${response.status}`)
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""
  while (true) {
    const { value, done } = await reader.read()
    if (done) return
    buffer += decoder.decode(value, { stream: true })
    let boundary = buffer.indexOf("\n\n")
    while (boundary >= 0) {
      const packet = buffer.slice(0, boundary)
      buffer = buffer.slice(boundary + 2)
      for (const line of packet.split("\n")) {
        if (!line.startsWith("data: ")) continue
        try { onEvent(JSON.parse(line.slice(6)) as ActivityEvent) } catch { continue }
      }
      boundary = buffer.indexOf("\n\n")
    }
  }
}

function formatTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString()
}
function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
