import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

export function ServersPage({ servers }: { servers: { name: string }[] }) {
  return <Card><CardHeader><CardTitle>MCP Servers</CardTitle></CardHeader><CardContent className="space-y-2">{servers.map(server => <div className="flex justify-between" key={server.name}><span>{server.name}</span><Badge>Ready</Badge></div>)}</CardContent></Card>
}
