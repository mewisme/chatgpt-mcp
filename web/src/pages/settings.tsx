import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { adminApi } from "@/lib/api"

export function SettingsPage() {
  async function save() { await adminApi.saveConfig({}) }

  return <Card><CardHeader><CardTitle>Settings</CardTitle></CardHeader><CardContent><Button onClick={save}>Save config</Button></CardContent></Card>
}
