import { useEffect, useState } from "react"

export function ActivityPage() {
  const [events, setEvents] = useState<string[]>([])

  useEffect(() => {
    const source = new EventSource("/api/activity/stream")
    source.onmessage = (event) => setEvents((items) => [event.data, ...items].slice(0, 100))
    return () => source.close()
  }, [])

  return <div className="space-y-2"><h1 className="text-xl font-semibold">Activity</h1>{events.map((event, index) => <div key={index} className="rounded border p-2 text-sm">{event}</div>)}</div>
}
