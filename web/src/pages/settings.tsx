import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { adminApi } from "@/lib/api"

export function SettingsPage() {
  const [config, setConfig] = useState<Record<string, unknown>>({})

  useEffect(() => { adminApi.config().then(setConfig) }, [])

  return <Card><CardHeader><CardTitle>Settings</CardTitle></CardHeader><CardContent className="space-y-4"><pre className="rounded-md border p-4 text-xs">{JSON.stringify(config, null, 2)}</pre><Button>Save</Button></CardContent></Card>
}
