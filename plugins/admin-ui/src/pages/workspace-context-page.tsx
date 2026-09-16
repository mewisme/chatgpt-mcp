import { lazy, Suspense } from "react"
import { useOutletContext } from "react-router-dom"
import { PageLoading } from "@/components/page-state"
import type { WorkspaceOutletContext } from "@/pages/workspace"

const WorkspaceContext = lazy(() => import("@/components/workspace-context").then((module) => ({ default: module.WorkspaceContext })))

export function WorkspaceContextPage() {
  const { workspace } = useOutletContext<WorkspaceOutletContext>()
  return <Suspense fallback={<PageLoading rows={5} />}><WorkspaceContext workspaceID={workspace.id} /></Suspense>
}
