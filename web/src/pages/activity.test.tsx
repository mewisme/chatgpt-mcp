import { act, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"
import { TooltipProvider } from "@/components/ui/tooltip"
import { ActivityPage } from "@/pages/activity"

let controller: ReadableStreamDefaultController<Uint8Array> | undefined

describe("activity page", () => {
  afterEach(() => { controller = undefined; vi.unstubAllGlobals() })

  it("pauses live events, resumes them, and opens event detail", async () => {
    const encoder = new TextEncoder()
    vi.stubGlobal("fetch", vi.fn(async () => new Response(new ReadableStream<Uint8Array>({ start(value) { controller = value; value.enqueue(encoder.encode('event: ready\ndata: {"latest_sequence":0}\n\n')) } }), { status: 200, headers: { "Content-Type": "text/event-stream" } })))
    const user = userEvent.setup()
    const view = render(<TooltipProvider><ActivityPage /></TooltipProvider>)
    expect(await screen.findByText("Live")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Pause" }))
    await act(async () => { controller?.enqueue(encoder.encode('event: activity\ndata: {"sequence":1,"kind":"tool_call","tool":"run_command","workspace_id":"ws_test","status":"success","duration_ms":12,"message":"Command completed","raw":{"method":"tools/call","source":"tunnel","tool":"run_command","arguments":{"workspace_id":"ws_test","working_directory":"/tmp/project","command":"go test ./..."},"params":{"name":"run_command","arguments":{"workspace_id":"ws_test","working_directory":"/tmp/project","command":"go test ./..."},"_meta":{"request_id":"req_test"}},"request":{"jsonrpc":"2.0","id":"call_1","method":"tools/call"},"result_type":"complete","status":"ok"},"timestamp":"2026-08-31T12:00:00Z"}\n\n')) })
    expect(await screen.findByRole("button", { name: "Resume (1)" })).toBeInTheDocument()
    expect(screen.queryByText("run_command")).not.toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Resume (1)" }))
    await user.click(await screen.findByText("run_command"))
    expect(await screen.findByRole("tab", { name: "Overview" })).toBeInTheDocument()
    await user.click(screen.getByRole("tab", { name: "Raw" }))
    expect(screen.getByRole("button", { name: "Copy JSON" })).toBeInTheDocument()
    expect(screen.getByText(/go test \.\/\.\.\./)).toBeInTheDocument()
    expect(screen.getByText(/req_test/)).toBeInTheDocument()
    expect(screen.getByText(/call_1/)).toBeInTheDocument()
    await act(async () => { controller?.close() })
    view.unmount()
  })
})
