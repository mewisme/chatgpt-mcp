import { lazy, Suspense } from "react"
import { useOutletContext } from "react-router-dom"
import { PageLoading } from "@/components/page-state"
import type { WorkspaceOutletContext } from "@/pages/workspace"

const RequestsPage = lazy(() => import("@/pages/requests").then((module) => ({ default: module.RequestsPage })))

export function WorkspaceRequestsPage() {
  const { workspace } = useOutletContext<WorkspaceOutletContext>()
  return <Suspense fallback={<PageLoading rows={5} />}><RequestsPage workspaceID={workspace.id} /></Suspense>
}
