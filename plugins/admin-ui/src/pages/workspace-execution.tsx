import { lazy, Suspense } from "react"
import { useOutletContext, useParams } from "react-router-dom"
import { PageLoading } from "@/components/page-state"
import type { WorkspaceOutletContext } from "@/pages/workspace"

const WorkspaceExecutionDetail = lazy(() => import("@/components/workspace-executions").then((module) => ({ default: module.WorkspaceExecutionDetail })))

export function WorkspaceExecutionPage() {
  const { workspace } = useOutletContext<WorkspaceOutletContext>()
  const { executionID = "" } = useParams<{ executionID: string }>()
  return <Suspense fallback={<PageLoading rows={5} />}><WorkspaceExecutionDetail workspaceID={workspace.id} executionID={executionID} /></Suspense>
}
