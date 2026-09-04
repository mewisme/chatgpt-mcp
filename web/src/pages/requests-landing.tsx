import { useEffect, useState } from "react"
import { FolderGit2, ShieldCheck } from "lucide-react"
import { useNavigate } from "react-router-dom"
import { PageEmpty, PageError, PageLoading } from "@/components/page-state"
import { PageHeader } from "@/components/page-header"
import { TruncatedText } from "@/components/truncated-text"
import { Item, ItemContent, ItemDescription, ItemGroup, ItemMedia, ItemTitle } from "@/components/ui/item"
import { adminApi, type Workspace } from "@/lib/api"

export function RequestsLandingPage() {
  const navigate = useNavigate()
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  useEffect(() => { let active = true; void adminApi.workspaces().then((value) => { if (active) { setWorkspaces(value); setError("") } }).catch((value) => { if (active) setError(errorText(value)) }).finally(() => { if (active) setLoading(false) }); return () => { active = false } }, [])
  return <div className="space-y-6"><PageHeader title="Approval requests" description="Select a workspace to review its control approvals and resolved request history." /><PageError message={error} />{loading ? <PageLoading rows={5} /> : workspaces.length === 0 ? <PageEmpty icon={ShieldCheck} title="No workspaces registered" description="Register a workspace before reviewing workspace-scoped approval requests." /> : <ItemGroup>{workspaces.map((workspace) => <Item className="cursor-pointer" key={workspace.id} role="button" tabIndex={0} variant="outline" onClick={() => navigate(`/workspaces/${encodeURIComponent(workspace.id)}/requests`)} onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") navigate(`/workspaces/${encodeURIComponent(workspace.id)}/requests`) }}><ItemMedia variant="icon"><FolderGit2 className="text-muted-foreground" /></ItemMedia><ItemContent className="min-w-0"><ItemTitle><TruncatedText lines={1}>{workspace.path}</TruncatedText></ItemTitle><ItemDescription className="font-mono">{workspace.id}</ItemDescription></ItemContent></Item>)}</ItemGroup>}</div>
}

function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }