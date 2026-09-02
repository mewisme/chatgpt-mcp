export function JsonViewer({ value, maxHeight = "24rem" }: { value: unknown; maxHeight?: string }) {
  const text = JSON.stringify(value ?? null, null, 2)
  return <pre className="m-0 min-w-0 max-w-full overflow-auto whitespace-pre rounded-lg border bg-background p-4 font-mono text-[13px] leading-relaxed" style={{ maxHeight }}><code>{text}</code></pre>
}
