import { Badge } from "@/components/ui/badge"

export function ServersPage({ servers }: { servers: { name: string }[] }) {
  return <div className="grid gap-2">{servers.map(server => <div className="flex items-center justify-between rounded-md border p-3" key={server.name}><span>{server.name}</span><Badge variant="secondary">Offline</Badge></div>)}</div>
}
