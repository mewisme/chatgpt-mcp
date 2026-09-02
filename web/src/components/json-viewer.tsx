import { Code, CodeBlock, CodeHeader } from "@/components/animate-ui/components/animate/code"

export function JsonViewer({ value, maxHeight = "24rem", filename = "data.json" }: { value: unknown; maxHeight?: string; filename?: string }) {
  const text = JSON.stringify(value ?? null, null, 2)
  return <Code code={text}><CodeHeader copyButton copyLabel="Copy JSON">{filename}</CodeHeader><CodeBlock lang="json" style={{ maxHeight }} /></Code>
}
