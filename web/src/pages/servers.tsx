import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

type Server = { id: string; name: string; transport: string; enabled: boolean }

export function ServersPage({ servers, onRemove }: { servers: Server[]; onAdd?: (server: Server) => void; onRemove?: (id: string) => void }) {
  return <Card><CardHeader><CardTitle>MCP Servers</CardTitle></CardHeader><CardContent className="space-y-4">{servers.map((server) => <div className="flex items-center justify-between" key={server.id}><div><div>{server.name}</div><div className="text-muted-foreground text-sm">{server.transport}</div></div><div className="flex items-center gap-2"><Badge>{server.enabled ? "Enabled" : "Disabled"}</Badge><Button variant="outline" size="sm" onClick={() => onRemove?.(server.id)}>Remove</Button></div></div>)}</CardContent></Card>
}
