import { CodeBlock, CodeBlockBody, CodeBlockContent, CodeBlockCopyButton, CodeBlockFilename, CodeBlockFiles, CodeBlockHeader, CodeBlockItem } from "@/components/kibo-ui/code-block"

export function JsonViewer({ value, maxHeight = "24rem", filename = "data.json" }: { value: unknown; maxHeight?: string; filename?: string }) {
  const text = JSON.stringify(value ?? null, null, 2)
  const data = [{ language: "json", filename, code: text }]
  return <CodeBlock data={data} defaultValue="json"><CodeBlockHeader><CodeBlockFiles>{(item) => <CodeBlockFilename key={item.filename} value={item.language}>{item.filename}</CodeBlockFilename>}</CodeBlockFiles><CodeBlockCopyButton aria-label="Copy JSON" /></CodeBlockHeader><CodeBlockBody className="overflow-y-auto" style={{ maxHeight }}>{(item) => <CodeBlockItem key={item.filename} value={item.language}><CodeBlockContent language="json">{item.code}</CodeBlockContent></CodeBlockItem>}</CodeBlockBody></CodeBlock>
}
