import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

export function ServersPage({ servers }: { servers: { name: string }[] }) {
  return <div className="grid gap-4">{servers.map((server) => <Card key={server.name}><CardHeader><CardTitle>{server.name}</CardTitle></CardHeader><CardContent>Ready</CardContent></Card>)}</div>
}
