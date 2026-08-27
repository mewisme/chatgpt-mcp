import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"

const items = [
  ["MCP Sessions", "0"],
  ["Workspaces", "0"],
  ["Tools", "0"],
  ["Upstream Servers", "0"],
]

export function App() {
  return <main className="min-h-svh p-6">
    <h1 className="text-2xl font-semibold">ChatGPT MCP Admin</h1>
    <Separator className="my-6" />
    <section className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
      {items.map(([title, value]) => <Card key={title}>
        <CardHeader><CardTitle className="text-sm">{title}</CardTitle></CardHeader>
        <CardContent className="text-3xl font-semibold">{value}</CardContent>
      </Card>)}
    </section>
  </main>
}

export default App
