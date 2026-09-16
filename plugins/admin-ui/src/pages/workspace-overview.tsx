import { FolderGit2 } from "lucide-react"
import { useOutletContext } from "react-router-dom"
import { CopyButton } from "@/components/copy-button"
import { DetailRow } from "@/components/detail-row"
import type { WorkspaceOutletContext } from "@/pages/workspace"

export function WorkspaceOverviewPage() {
  const { workspace } = useOutletContext<WorkspaceOutletContext>()
  return <div className="rounded-xl border bg-card"><div className="flex items-center gap-3 border-b p-4"><div className="flex size-9 items-center justify-center rounded-lg bg-muted"><FolderGit2 className="size-4 text-muted-foreground" /></div><div className="min-w-0"><div className="truncate text-sm font-medium">{workspace.path}</div><div className="font-mono text-xs text-muted-foreground">{workspace.id}</div></div></div><div className="divide-y px-4"><DetailRow label="Path" value={<CopyValue value={workspace.path} />} /><DetailRow label="Workspace ID" value={<CopyValue value={workspace.id} />} /><DetailRow label="Allowed directories" value={workspace.allow_dirs?.length ? <div className="space-y-1">{workspace.allow_dirs.map((value) => <CopyValue key={value} value={value} />)}</div> : "None"} /></div></div>
}

function CopyValue({ value }: { value: string }) { return <div className="flex min-w-0 items-start gap-1"><span className="min-w-0 flex-1 break-all font-mono text-sm">{value}</span><CopyButton value={value} /></div> }
