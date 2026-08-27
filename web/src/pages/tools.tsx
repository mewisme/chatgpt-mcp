import { Badge } from "@/components/ui/badge"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

type Tool = { name: string; description?: string }

export function ToolsPage({ tools }: { tools: Tool[] }) {
  return <Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Description</TableHead></TableRow></TableHeader><TableBody>{tools.map(tool => <TableRow key={tool.name}><TableCell><Badge>{tool.name}</Badge></TableCell><TableCell>{tool.description}</TableCell></TableRow>)}</TableBody></Table>
}
