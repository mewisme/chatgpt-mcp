import { Sidebar, SidebarContent, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar"
import { navItems } from "@/lib/admin-navigation"

export function AppSidebar({ page, onPageChange }: { page: string; onPageChange: (page: string) => void }) {
  return <Sidebar><SidebarContent><SidebarGroup><SidebarGroupLabel className="h-auto py-3"><span><span className="block font-semibold text-foreground">chatgpt-mcp</span><span className="block text-[11px] font-normal text-muted-foreground">Admin dashboard</span></span></SidebarGroupLabel><SidebarGroupContent><SidebarMenu>{navItems.map((item) => <SidebarMenuItem key={item.id}><SidebarMenuButton isActive={page === item.id} tooltip={item.title} onClick={() => onPageChange(item.id)}><item.icon /><span>{item.title}</span></SidebarMenuButton></SidebarMenuItem>)}</SidebarMenu></SidebarGroupContent></SidebarGroup></SidebarContent></Sidebar>
}
