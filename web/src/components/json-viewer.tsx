import { CopyButton } from "@/components/copy-button"
import { ScrollArea } from "@/components/ui/scroll-area"

export function JsonViewer({ value, maxHeight = "24rem" }: { value: unknown; maxHeight?: string }) {
  const text = JSON.stringify(value ?? null, null, 2)
  return <div className="overflow-hidden rounded-lg border bg-muted/20"><div className="flex items-center justify-between border-b px-3 py-1.5"><span className="text-xs font-medium text-muted-foreground">JSON</span><CopyButton label="Copy JSON" value={text} /></div><ScrollArea style={{ maxHeight }}><pre className="min-w-0 whitespace-pre-wrap break-words p-3 font-mono text-xs leading-relaxed">{text}</pre></ScrollArea></div>
}