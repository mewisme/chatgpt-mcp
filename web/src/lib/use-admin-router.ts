import { useLocation, useNavigate } from "react-router-dom"
import { adminRouteFromPath } from "@/lib/admin-route"

export function useAdminRouter() {
  const location = useLocation()
  const routerNavigate = useNavigate()
  const route = adminRouteFromPath(location.pathname)
  return { route, navigate: (path: string, options?: { replace?: boolean }) => routerNavigate(adminRouteFromPath(path).path, { replace: options?.replace }) }
}