import { authHeaders, type ExecutionEvent, type ExecutionSnapshot } from "@/lib/api"

export type ExecutionStreamHandlers = {
  onSnapshot?: (snapshot: ExecutionSnapshot) => void
  onEvent?: (event: ExecutionEvent) => void
}

export async function streamExecution(workspaceID: string, executionID: string, signal: AbortSignal, handlers: ExecutionStreamHandlers = {}) {
  const path = `/api/workspaces/${encodeURIComponent(workspaceID)}/executions/${encodeURIComponent(executionID)}/stream`
  const response = await fetch(path, { headers: authHeaders(), signal })
  if (!response.ok || !response.body) {
    const message = await response.text().catch(() => "")
    throw new Error(message.trim() || `Execution stream ${response.status}`)
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""
  let complete = false
  while (true) {
    const { value, done } = await reader.read()
    if (done) {
      if (signal.aborted || complete) return
      throw new Error("Execution stream ended; reconnecting from the latest snapshot.")
    }
    buffer += decoder.decode(value, { stream: true })
    let boundary = buffer.indexOf("\n\n")
    while (boundary >= 0) {
      const packet = buffer.slice(0, boundary)
      buffer = buffer.slice(boundary + 2)
      let eventType = "message"
      let data = ""
      for (const line of packet.split("\n")) {
        if (line.startsWith("event: ")) eventType = line.slice(7).trim()
        if (line.startsWith("data: ")) data += line.slice(6)
      }
      if (eventType === "overflow") throw new Error("Execution stream overflowed; reconnecting from the latest snapshot.")
      if (eventType === "ready" && data) {
        const snapshot = JSON.parse(data) as ExecutionSnapshot
        handlers.onSnapshot?.(snapshot)
        if (snapshot.execution.status !== "running") complete = true
      } else if ((eventType === "output" || eventType === "completed") && data) {
        const event = JSON.parse(data) as ExecutionEvent
        handlers.onEvent?.(event)
        if (event.type === "completed") complete = true
      }
      if (complete) return
      boundary = buffer.indexOf("\n\n")
    }
  }
}