import { useContext } from "react"
import { AdminRouterContext } from "@/lib/admin-router-context"

export function useAdminRouter() { const value = useContext(AdminRouterContext); if (!value) throw new Error("AdminRouterProvider is required"); return value }