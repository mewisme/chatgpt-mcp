import { useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { adminApi, type Tool } from "@/lib/api"

export function ToolsPage() {
  const [tools, setTools] = useState<Tool[]>([])
  const [error, setError] = useState("")
  useEffect(() => { void adminApi.tools().then(setTools).catch((value) => setError(value instanceof Error ? value.message : String(value))) }, [])
  return <div className="space-y-3">{error ? <div className="text-sm text-destructive">{error}</div> : null}<Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Description</TableHead></TableRow></TableHeader><TableBody>{tools.map((tool) => <TableRow key={tool.name}><TableCell><Badge>{tool.name}</Badge></TableCell><TableCell>{tool.description}</TableCell></TableRow>)}</TableBody></Table></div>
}
