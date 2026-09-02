import { CopyButton } from "@/components/copy-button"
import { ScrollArea } from "@/components/ui/scroll-area"

export function JsonViewer({ value, maxHeight = "24rem", filename = "data.json" }: { value: unknown; maxHeight?: string; filename?: string }) {
  const text = JSON.stringify(value ?? null, null, 2)
  return <div className="min-w-0 max-w-full overflow-hidden rounded-lg border bg-background"><div className="flex h-10 min-w-0 items-center border-b bg-muted/50 px-3"><span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">{filename}</span><CopyButton label="Copy JSON" value={text} /></div><ScrollArea className="min-w-0 max-w-full overflow-hidden [&_[data-slot=scroll-area-viewport]>div]:!block [&_[data-slot=scroll-area-viewport]>div]:!w-full [&_[data-slot=scroll-area-viewport]>div]:!min-w-0 [&_[data-slot=scroll-area-viewport]>div]:!max-w-full" style={{ maxHeight }}><pre className="m-0 w-full min-w-0 max-w-full whitespace-pre-wrap break-words p-4 font-mono text-[13px] leading-relaxed [overflow-wrap:anywhere]"><code className="whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{text}</code></pre></ScrollArea></div>
}
