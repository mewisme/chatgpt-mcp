import { ScrollArea } from "@/components/ui/scroll-area"

export function JsonViewer({ value, maxHeight = "24rem" }: { value: unknown; maxHeight?: string }) {
  const text = JSON.stringify(value ?? null, null, 2)
  return <div className="min-w-0 max-w-full rounded-lg border bg-background"><ScrollArea className="min-w-0 max-w-full" style={{ maxHeight }}><pre className="m-0 w-max min-w-full whitespace-pre p-4 font-mono text-[13px] leading-relaxed"><code>{text}</code></pre></ScrollArea></div>
}
