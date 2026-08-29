import { useEffect, useMemo, useState } from "react"
import { RefreshCw, Search, Wrench } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { adminApi, type Tool } from "@/lib/api"

export function ToolsPage() {
  const [tools, setTools] = useState<Tool[]>([])
  const [query, setQuery] = useState("")
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  async function load(manual = false) {
    if (manual) setBusy(true)
    try { setTools(await adminApi.tools()); setError("") } catch (value) { setError(errorText(value)) } finally { setLoading(false); setBusy(false) }
  }

  useEffect(() => {
    let active = true
    void adminApi.tools().then((next) => { if (active) { setTools(next); setError("") } }).catch((value) => { if (active) setError(errorText(value)) }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return tools
    return tools.filter((tool) => [tool.name, tool.title, tool.description].some((value) => value?.toLowerCase().includes(needle)))
  }, [query, tools])

  return <Card><CardHeader className="gap-3 sm:flex-row sm:items-center sm:justify-between"><div><CardTitle className="flex items-center gap-2"><Wrench className="size-4" />Tool catalog <Badge variant="secondary">{tools.length}</Badge></CardTitle><div className="mt-1 text-sm text-muted-foreground">Tools exposed by the local runtime and enabled upstream servers.</div></div><Button disabled={busy} size="sm" variant="outline" onClick={() => void load(true)}><RefreshCw className={busy ? "animate-spin" : ""} />Refresh</Button></CardHeader><CardContent className="space-y-4"><div className="relative"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input className="pl-9" placeholder="Search by name or description..." value={query} onChange={(event) => setQuery(event.target.value)} /></div>{error ? <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">{error}</div> : null}{loading ? <div className="py-10 text-center text-sm text-muted-foreground">Loading tools...</div> : filtered.length === 0 ? <div className="rounded-lg border border-dashed py-10 text-center"><div className="text-sm font-medium">{tools.length === 0 ? "No tools exposed" : "No matching tools"}</div><div className="mt-1 text-xs text-muted-foreground">{tools.length === 0 ? "Register a workspace or enable an upstream server to expose tools." : "Try a different search term."}</div></div> : <div className="overflow-hidden rounded-lg border"><Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Description</TableHead><TableHead className="w-[180px]">Hints</TableHead></TableRow></TableHeader><TableBody>{filtered.map((tool) => <ToolRow key={tool.name} tool={tool} />)}</TableBody></Table></div>}</CardContent></Card>
}

function ToolRow({ tool }: { tool: Tool }) {
  const readOnly = tool.annotations?.readOnlyHint === true
  const destructive = tool.annotations?.destructiveHint === true
  return <TableRow><TableCell className="align-top"><div className="font-mono text-sm font-medium">{tool.name}</div>{tool.title && tool.title !== tool.name ? <div className="mt-1 text-xs text-muted-foreground">{tool.title}</div> : null}</TableCell><TableCell className="max-w-xl align-top text-sm text-muted-foreground">{tool.description || "No description."}</TableCell><TableCell className="align-top"><div className="flex flex-wrap gap-1">{readOnly ? <Badge variant="outline">Read only</Badge> : <Badge variant="secondary">Writes</Badge>}{destructive ? <Badge variant="destructive">Destructive</Badge> : null}</div></TableCell></TableRow>
}

function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
