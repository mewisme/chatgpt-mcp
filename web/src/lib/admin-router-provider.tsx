import type { ReactNode } from "react"
import { AdminRouterContext, type AdminRouterValue } from "@/lib/admin-router-context"

export function AdminRouterProvider({ route, navigate, children }: AdminRouterValue & { children: ReactNode }) { return <AdminRouterContext.Provider value={{ route, navigate }}>{children}</AdminRouterContext.Provider> }