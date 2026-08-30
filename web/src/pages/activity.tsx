import { useEffect, useMemo, useState } from "react"
import { CircleDot, Trash2 } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { authHeaders } from "@/lib/api"

type ActivityEvent = {
  sequence?: number
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
    void streamActivity(controller.signal, {
      onReady: () => setConnected(true),
      onEvent: (event) => { setConnected(true); setEvents((items) => [event, ...items].slice(0, 200)) },
      onGap: (from, to) => setError(`Activity stream skipped ${to - from - 1} event(s) between sequence ${from} and ${to}.`),
    }).then(() => {
      if (!controller.signal.aborted) {
        setConnected(false)
        setError("Activity stream closed; reload to reconnect.")
      }
    }).catch((value) => {
      if (!controller.signal.aborted) {
        setConnected(false)
        setError(errorText(value))
      }
    })
    return () => controller.abort()
  }, [])

  const kinds = useMemo(() => [...new Set(events.map((event) => event.kind))].sort(), [events])
  const statuses = useMemo(() => [...new Set(events.map((event) => event.status).filter((value): value is string => Boolean(value)))].sort(), [events])
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return events.filter((event) => {
      if (kind !== "all" && event.kind !== kind) return false
      if (status !== "all" && event.status !== status) return false
      if (!needle) return true
      return [event.kind, event.method, event.source, event.tool, event.workspace_id, event.status, event.message].some((value) => value?.toLowerCase().includes(needle))
    })
  }, [events, kind, status, query])

  return <div className="space-y-6"><Card><CardHeader><div className="flex flex-wrap items-center justify-between gap-3"><div><CardTitle>Activity</CardTitle><CardDescription>Live MCP requests, tool calls, and runtime lifecycle events. The stream replays recent in-memory events when this page opens.</CardDescription></div><Badge variant={connected ? "secondary" : "outline"}><CircleDot className="mr-1 size-3" />{connected ? "Live" : "Connecting"}</Badge></div></CardHeader><CardContent className="space-y-4"><div className="flex flex-col gap-2 lg:flex-row"><Input className="lg:flex-1" placeholder="Filter tool, workspace, message..." value={query} onChange={(event) => setQuery(event.target.value)} /><Select value={kind} onValueChange={setKind}><SelectTrigger className="w-full lg:w-48"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">All event types</SelectItem>{kinds.map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select><Select value={status} onValueChange={setStatus}><SelectTrigger className="w-full lg:w-40"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">All statuses</SelectItem>{statuses.map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select><Button variant="outline" onClick={() => setEvents([])}><Trash2 />Clear</Button></div>{error ? <div className="text-sm text-destructive">{error}</div> : null}<div className="space-y-2">{filtered.length === 0 ? <div className="text-sm text-muted-foreground">No matching activity.</div> : filtered.map((event, index) => <ActivityRow event={event} key={`${event.sequence ?? event.timestamp}-${event.kind}-${event.tool || event.method || ""}-${index}`} />)}</div></CardContent></Card></div>
}

function ActivityRow({ event }: { event: ActivityEvent }) {
  const title = event.tool || event.method || event.kind
  const destructive = event.status === "error" || event.status === "degraded"
  return <div className="rounded-lg border p-3"><div className="flex flex-wrap items-center gap-2"><div className="font-mono text-sm font-medium">{title}</div><Badge variant="outline">{event.kind}</Badge>{event.status ? <Badge variant={destructive ? "destructive" : "secondary"}>{event.status}</Badge> : null}<div className="ml-auto text-xs text-muted-foreground">{formatTime(event.timestamp)}</div></div><div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">{event.sequence !== undefined ? <span>seq {event.sequence}</span> : null}{event.source ? <span>source {event.source}</span> : null}{event.workspace_id ? <span>workspace {event.workspace_id}</span> : null}{event.duration_ms !== undefined ? <span>{event.duration_ms} ms</span> : null}</div>{event.message ? <div className="mt-2 break-words text-sm text-muted-foreground">{event.message}</div> : null}</div>
}

type ActivityStreamHandlers = { onReady: () => void; onEvent: (event: ActivityEvent) => void; onGap: (from: number, to: number) => void }

async function streamActivity(signal: AbortSignal, handlers: ActivityStreamHandlers) {
  const response = await fetch("/api/activity/stream?history=100", { headers: authHeaders(), signal })
  if (!response.ok || !response.body) {
    const message = await response.text().catch(() => "")
    throw new Error(message.trim() || `Activity stream ${response.status}`)
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""
  let lastSequence = 0
  while (true) {
    const { value, done } = await reader.read()
    if (done) return
    buffer += decoder.decode(value, { stream: true })
    let boundary = buffer.indexOf("\n\n")
    while (boundary >= 0) {
      const packet = buffer.slice(0, boundary)
      buffer = buffer.slice(boundary + 2)
      let eventType = "message"
      let data = ""
      for (const line of packet.split("\n")) {
        if (line.startsWith("event: ")) eventType = line.slice(7).trim()
        if (line.startsWith("data: ")) data += line.slice(6)
      }
      if (eventType === "ready" || eventType === "heartbeat") handlers.onReady()
      else if (eventType === "overflow") throw new Error("Activity stream subscriber overflowed; reconnect to resync recent events.")
      else if (eventType === "activity" && data) {
        try {
          const event = JSON.parse(data) as ActivityEvent
          const sequence = event.sequence ?? 0
          if (lastSequence > 0 && sequence > lastSequence + 1) handlers.onGap(lastSequence, sequence)
          if (sequence > 0) lastSequence = sequence
          handlers.onEvent(event)
        } catch (value) {
          if (value instanceof SyntaxError) continue
          throw value
        }
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
