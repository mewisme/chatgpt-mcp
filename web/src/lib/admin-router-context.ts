import { createContext } from "react"
import type { AdminRoute } from "@/lib/admin-route"

export type AdminRouterValue = { route: AdminRoute; navigate: (path: string, options?: { replace?: boolean }) => void }
export const AdminRouterContext = createContext<AdminRouterValue | null>(null)