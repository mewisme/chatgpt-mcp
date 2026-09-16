import { lazy, Suspense } from "react"
import { Outlet } from "react-router-dom"
import { AppSidebar } from "@/components/app-sidebar"
import { PageLoading } from "@/components/page-state"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import type { AdminRouteHandle } from "@/lib/admin-navigation"

const RequestApprovalHost = lazy(() => import("@/components/request-approval-host").then((module) => ({ default: module.RequestApprovalHost })))

export function AuthenticatedShell({ route, authRequired, onSignOut }: {
  route: AdminRouteHandle
  authRequired: boolean
  onSignOut: () => void
}) {
  return <TooltipProvider><SidebarProvider><AppSidebar authRequired={authRequired} onSignOut={onSignOut} /><SidebarInset className="min-w-0"><header className="sticky top-0 z-20 flex min-h-14 items-center gap-3 border-b bg-background/95 px-3 backdrop-blur supports-[backdrop-filter]:bg-background/80 sm:px-4"><SidebarTrigger /><div className="min-w-0 flex-1"><div className="truncate text-sm font-semibold">{route.title}</div><div className="hidden truncate text-xs text-muted-foreground sm:block">{route.description}</div></div></header><div className="min-w-0 flex-1 bg-muted/20"><div className="mx-auto w-full max-w-[1400px] p-4 sm:p-6 lg:p-8"><Suspense fallback={<PageLoading rows={6} />}><Outlet /></Suspense></div></div></SidebarInset><Suspense fallback={null}><RequestApprovalHost /></Suspense><Toaster /></SidebarProvider></TooltipProvider>
}
