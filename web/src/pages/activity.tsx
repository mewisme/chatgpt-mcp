import { useEffect, useMemo, useRef, useState } from "react"
import { createColumnHelper } from "@tanstack/react-table"
import { CircleDot, Pause, Play, Radio, Search, Trash2 } from "lucide-react"
import { DataTable } from "@/components/data-table"
import { DataTableColumnHeader } from "@/components/data-table-column-header"
import type { DataTableFeatures } from "@/components/data-table-features"
import { DetailRow } from "@/components/detail-row"
import { JsonViewer } from "@/components/json-viewer"
import { PageError, PageEmpty } from "@/components/page-state"
import { PageHeader } from "@/components/page-header"
import { ResponsiveDialog } from "@/components/responsive-dialog"
import { TruncatedText } from "@/components/truncated-text"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Item, ItemContent, ItemDescription, ItemGroup, ItemHeader, ItemTitle } from "@/components/ui/item"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useIsMobile } from "@/hooks/use-mobile"
import { authHeaders } from "@/lib/api"

export type ActivityEvent = { sequence?: number; kind: string; method?: string; source?: string; tool?: string; workspace_id?: string; status?: string; duration_ms?: number; message?: string; raw?: Record<string, unknown>; timestamp: string }
type ActivityStreamHandlers = { onReady: () => void; onEvent: (event: ActivityEvent) => void; onGap: (from: number, to: number) => void }

const columnHelper = createColumnHelper<DataTableFeatures, ActivityEvent>()
const columns = columnHelper.columns([
  columnHelper.accessor("timestamp", { header: ({ column }) => <DataTableColumnHeader column={column} title="Time" />, cell: ({ getValue }) => <span className="whitespace-nowrap text-xs text-muted-foreground">{formatTime(getValue())}</span> }),
  columnHelper.display({ id: "event", header: "Event", cell: ({ row }) => <div className="min-w-0"><TruncatedText className="font-mono text-sm font-medium" lines={1}>{activityTitle(row.original)}</TruncatedText><TruncatedText className="mt-1 text-xs text-muted-foreground" lines={1}>{row.original.kind}</TruncatedText></div> }),
  columnHelper.accessor("workspace_id", { header: "Workspace", cell: ({ getValue }) => getValue() ? <TruncatedText className="text-xs text-muted-foreground" lines={1} mono>{getValue()!}</TruncatedText> : <span className="text-muted-foreground">-</span> }),
  columnHelper.accessor("status", { header: "Status", cell: ({ getValue }) => getValue() ? <StatusBadge status={getValue()!} /> : <span className="text-muted-foreground">-</span> }),
  columnHelper.accessor("duration_ms", { header: ({ column }) => <DataTableColumnHeader column={column} title="Duration" />, cell: ({ getValue }) => <span className="whitespace-nowrap text-xs text-muted-foreground">{getValue() === undefined ? "-" : formatDuration(getValue()!)}</span> }),
  columnHelper.accessor("source", { header: "Source", cell: ({ getValue }) => <span className="text-xs text-muted-foreground">{getValue() || "-"}</span> }),
])

export function ActivityPage() {
  const mobile = useIsMobile()
  const [events, setEvents] = useState<ActivityEvent[]>([])
  const [pending, setPending] = useState<ActivityEvent[]>([])
  const [selected, setSelected] = useState<ActivityEvent | null>(null)
  const [kind, setKind] = useState("all")
  const [status, setStatus] = useState("all")
  const [source, setSource] = useState("all")
  const [query, setQuery] = useState("")
  const [paused, setPaused] = useState(false)
  const pausedRef = useRef(false)
  const [connected, setConnected] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => { pausedRef.current = paused }, [paused])
  useEffect(() => {
    const controller = new AbortController()
    void streamActivity(controller.signal, {
      onReady: () => setConnected(true),
      onEvent: (event) => { setConnected(true); if (pausedRef.current) setPending((items) => [event, ...items].slice(0, 200)); else setEvents((items) => [event, ...items].slice(0, 200)) },
      onGap: (from, to) => setError(`Activity stream skipped ${to - from - 1} event(s) between sequence ${from} and ${to}.`),
    }).then(() => {
      if (!controller.signal.aborted) { setConnected(false); setError("Activity stream closed; reload to reconnect.") }
    }).catch((value) => {
      if (!controller.signal.aborted) { setConnected(false); setError(errorText(value)) }
    })
    return () => controller.abort()
  }, [])

  const kinds = useMemo(() => unique(events, pending, (event) => event.kind), [events, pending])
  const statuses = useMemo(() => unique(events, pending, (event) => event.status), [events, pending])
  const sources = useMemo(() => unique(events, pending, (event) => event.source), [events, pending])
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return events.filter((event) => {
      if (kind !== "all" && event.kind !== kind) return false
      if (status !== "all" && event.status !== status) return false
      if (source !== "all" && event.source !== source) return false
      if (!needle) return true
      return [event.kind, event.method, event.source, event.tool, event.workspace_id, event.status, event.message].some((value) => value?.toLowerCase().includes(needle))
    })
  }, [events, kind, query, source, status])

  function resume() { setEvents((items) => [...pending, ...items].slice(0, 200)); setPending([]); setPaused(false) }
  function clear() { setEvents([]); setPending([]) }

  return <div className="space-y-6"><PageHeader title="Activity" description="Live MCP requests, tool calls, and runtime lifecycle events. Open any event to inspect its complete metadata." actions={<><Badge variant={connected ? "secondary" : "outline"}><CircleDot className="size-3" />{connected ? "Live" : "Connecting"}</Badge><Button size="sm" variant="outline" onClick={() => paused ? resume() : setPaused(true)}>{paused ? <Play /> : <Pause />}{paused ? `Resume${pending.length ? ` (${pending.length})` : ""}` : "Pause"}</Button><Button size="sm" variant="outline" onClick={clear}><Trash2 />Clear</Button></>} /><PageError message={error} /><div className="flex flex-col gap-2 lg:flex-row"><div className="relative min-w-0 flex-1"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input className="pl-9" placeholder="Search tool, workspace, source, message..." value={query} onChange={(event) => setQuery(event.target.value)} /></div><FilterSelect label="All event types" value={kind} values={kinds} onValueChange={setKind} /><FilterSelect label="All statuses" value={status} values={statuses} onValueChange={setStatus} /><FilterSelect label="All sources" value={source} values={sources} onValueChange={setSource} /></div>{paused && pending.length ? <div className="rounded-lg border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">{pending.length} new event{pending.length === 1 ? "" : "s"} waiting while paused.</div> : null}{filtered.length === 0 ? <PageEmpty icon={Radio} title="No matching activity" description={events.length ? "Adjust the filters or resume the live stream." : "Activity will appear here as MCP requests and tools run."} /> : mobile ? <ActivityMobileList events={filtered} onSelect={setSelected} /> : <DataTable columns={columns} data={filtered} onRowClick={setSelected} pageSize={20} />}{selected ? <ActivityDetail event={selected} open onOpenChange={(open) => { if (!open) setSelected(null) }} /> : null}</div>
}

function ActivityMobileList({ events, onSelect }: { events: ActivityEvent[]; onSelect: (event: ActivityEvent) => void }) {
  return <ItemGroup>{events.map((event, index) => <Item className="cursor-pointer" key={`${event.sequence ?? event.timestamp}-${event.kind}-${index}`} role="button" tabIndex={0} variant="outline" onClick={() => onSelect(event)} onKeyDown={(value) => { if (value.key === "Enter" || value.key === " ") onSelect(event) }}><ItemContent><ItemHeader><ItemTitle className="min-w-0 font-mono">{activityTitle(event)}</ItemTitle>{event.status ? <StatusBadge status={event.status} /> : null}</ItemHeader><ItemDescription>{event.message || [event.kind, event.source, event.workspace_id].filter(Boolean).join(" · ")}</ItemDescription><div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground"><span>{formatTime(event.timestamp)}</span><span>{event.kind}</span>{event.duration_ms !== undefined ? <span>{formatDuration(event.duration_ms)}</span> : null}</div></ItemContent></Item>)}</ItemGroup>
}

function ActivityDetail({ event, open, onOpenChange }: { event: ActivityEvent; open: boolean; onOpenChange: (open: boolean) => void }) {
  return <ResponsiveDialog open={open} onOpenChange={onOpenChange} title={activityTitle(event)} description={`${event.kind} · ${formatDateTime(event.timestamp)}`}><Tabs defaultValue="overview"><TabsList className="w-full"><TabsTrigger value="overview">Overview</TabsTrigger><TabsTrigger value="metadata">Metadata</TabsTrigger><TabsTrigger value="raw">Raw</TabsTrigger></TabsList><TabsContent className="mt-4 divide-y" value="overview"><DetailRow label="Status" value={event.status ? <StatusBadge status={event.status} /> : "-"} /><DetailRow label="Tool" value={event.tool || "-"} mono /><DetailRow label="Method" value={event.method || "-"} mono /><DetailRow label="Message" value={event.message || "-"} /><DetailRow label="Duration" value={event.duration_ms === undefined ? "-" : formatDuration(event.duration_ms)} /></TabsContent><TabsContent className="mt-4 divide-y" value="metadata"><DetailRow label="Sequence" value={event.sequence ?? "-"} mono /><DetailRow label="Timestamp" value={formatDateTime(event.timestamp)} /><DetailRow label="Source" value={event.source || "-"} mono /><DetailRow label="Workspace" value={event.workspace_id || "-"} mono /><DetailRow label="Kind" value={event.kind} mono /></TabsContent><TabsContent className="mt-4" value="raw"><JsonViewer filename="activity.json" value={event.raw ?? event} /></TabsContent></Tabs></ResponsiveDialog>
}

function FilterSelect({ label, value, values, onValueChange }: { label: string; value: string; values: string[]; onValueChange: (value: string) => void }) {
  return <Select value={value} onValueChange={onValueChange}><SelectTrigger className="w-full lg:w-44"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">{label}</SelectItem>{values.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent></Select>
}

function StatusBadge({ status }: { status: string }) {
  const destructive = status === "error" || status === "degraded" || status === "failed"
  return <Badge variant={destructive ? "destructive" : status === "success" || status === "completed" || status === "ok" ? "secondary" : "outline"}>{status}</Badge>
}

function unique(events: ActivityEvent[], pending: ActivityEvent[], pick: (event: ActivityEvent) => string | undefined) { return [...new Set([...events, ...pending].map(pick).filter((value): value is string => Boolean(value)))].sort() }
function activityTitle(event: ActivityEvent) { return event.tool || event.method || event.kind }
function formatDuration(value: number) { return value < 1000 ? `${value} ms` : `${(value / 1000).toFixed(value < 10000 ? 1 : 0)} s` }
function formatTime(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString() }
function formatDateTime(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString() }
function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }

async function streamActivity(signal: AbortSignal, handlers: ActivityStreamHandlers) {
  const response = await fetch("/api/activity/stream?history=100", { headers: authHeaders(), signal })
  if (!response.ok || !response.body) { const message = await response.text().catch(() => ""); throw new Error(message.trim() || `Activity stream ${response.status}`) }
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
      for (const line of packet.split("\n")) { if (line.startsWith("event: ")) eventType = line.slice(7).trim(); if (line.startsWith("data: ")) data += line.slice(6) }
      if (eventType === "ready" || eventType === "heartbeat") handlers.onReady()
      else if (eventType === "overflow") throw new Error("Activity stream subscriber overflowed; reconnect to resync recent events.")
      else if (eventType === "activity" && data) {
        try {
          const event = JSON.parse(data) as ActivityEvent
          const sequence = event.sequence ?? 0
          if (lastSequence > 0 && sequence > lastSequence + 1) handlers.onGap(lastSequence, sequence)
          if (sequence > 0) lastSequence = sequence
          handlers.onEvent(event)
        } catch (value) { if (value instanceof SyntaxError) continue; throw value }
      }
      boundary = buffer.indexOf("\n\n")
    }
  }
}
