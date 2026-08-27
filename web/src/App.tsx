import { Activity, FolderGit2, Network, Settings, Terminal, Wrench } from "lucide-react"

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"

const navigation = [
  { name: "Overview", icon: Activity },
  { name: "Workspaces", icon: FolderGit2 },
  { name: "MCP Servers", icon: Network },
  { name: "Tools", icon: Wrench },
  { name: "Shell", icon: Terminal },
  { name: "Settings", icon: Settings },
]

const stats = [
  ["MCP Sessions", "0"],
  ["Workspaces", "0"],
  ["Upstream Servers", "0"],
  ["Active Tools", "0"],
]

export function App() {
  return (
    <div className="flex min-h-svh bg-background">
      <aside className="w-64 border-r p-4">
        <h1 className="mb-6 text-lg font-semibold">ChatGPT MCP</h1>
        <div className="space-y-1">
          {navigation.map((item) => {
            const Icon = item.icon
            return <button className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-muted" key={item.name}><Icon className="size-4" />{item.name}</button>
          })}
        </div>
      </aside>
      <main className="flex-1 p-6">
        <h2 className="text-2xl font-semibold">Overview</h2>
        <p className="text-sm text-muted-foreground">Workspace-scoped MCP runtime administration.</p>
        <Separator className="my-6" />
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          {stats.map(([name, value]) => <Card key={name}><CardHeader><CardTitle className="text-sm">{name}</CardTitle></CardHeader><CardContent className="text-3xl font-semibold">{value}</CardContent></Card>)}
        </div>
      </main>
    </div>
  )
}

export default App
