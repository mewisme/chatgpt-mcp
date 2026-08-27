import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"

export function SettingsPage() {
  return <Card><CardHeader><CardTitle>Settings</CardTitle></CardHeader><CardContent className="space-y-4"><Input placeholder="MCP token" /><Button>Save</Button></CardContent></Card>
}
