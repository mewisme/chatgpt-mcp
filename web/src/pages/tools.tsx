import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

export function ToolsPage({ tools }: { tools: string[] }) {
  return <Table><TableHeader><TableRow><TableHead>Name</TableHead></TableRow></TableHeader><TableBody>{tools.map(tool => <TableRow key={tool}><TableCell>{tool}</TableCell></TableRow>)}</TableBody></Table>
}
