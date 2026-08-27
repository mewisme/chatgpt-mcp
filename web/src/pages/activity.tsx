import { useEffect, useState } from "react"
import { authHeaders } from "@/lib/api"

export function ActivityPage() {
  const [events, setEvents] = useState<string[]>([])
  const [error, setError] = useState("")

  useEffect(() => {
    const controller = new AbortController()
    void streamActivity(controller.signal, (event) => setEvents((items) => [event, ...items].slice(0, 100))).catch((value) => {
      if (!controller.signal.aborted) setError(value instanceof Error ? value.message : String(value))
    })
    return () => controller.abort()
  }, [])

  return <div className="space-y-2"><h1 className="text-xl font-semibold">Activity</h1>{error ? <div className="text-sm text-destructive">{error}</div> : null}{events.map((event, index) => <div key={index} className="rounded border p-2 text-sm">{event}</div>)}</div>
}

async function streamActivity(signal: AbortSignal, onEvent: (event: string) => void) {
  const response = await fetch("/api/activity/stream", { headers: authHeaders(), signal })
  if (!response.ok || !response.body) throw new Error(`Activity stream ${response.status}`)
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""
  while (true) {
    const { value, done } = await reader.read()
    if (done) return
    buffer += decoder.decode(value, { stream: true })
    let boundary = buffer.indexOf("\n\n")
    while (boundary >= 0) {
      const packet = buffer.slice(0, boundary)
      buffer = buffer.slice(boundary + 2)
      for (const line of packet.split("\n")) if (line.startsWith("data: ")) onEvent(line.slice(6))
      boundary = buffer.indexOf("\n\n")
    }
  }
}
