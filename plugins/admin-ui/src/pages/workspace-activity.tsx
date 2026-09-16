import { lazy, Suspense } from "react"
import { useOutletContext } from "react-router-dom"
import { PageLoading } from "@/components/page-state"
import type { WorkspaceOutletContext } from "@/pages/workspace"

const WorkspaceExecutions = lazy(() => import("@/components/workspace-executions").then((module) => ({ default: module.WorkspaceExecutions })))

export function WorkspaceActivityPage() {
  const { workspace } = useOutletContext<WorkspaceOutletContext>()
  return <Suspense fallback={<PageLoading rows={5} />}><WorkspaceExecutions workspaceID={workspace.id} /></Suspense>
}
