import { Activity, Cloud, Home, Server, Settings, Wrench } from "lucide-react"
import { Sidebar, SidebarContent, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar"

const items = [{ id: "overview", title: "Overview", icon: Home }, { id: "tools", title: "Tools", icon: Wrench }, { id: "servers", title: "MCP Servers", icon: Server }, { id: "tunnel", title: "Tunnel", icon: Cloud }, { id: "activity", title: "Activity", icon: Activity }, { id: "settings", title: "Settings", icon: Settings }]

export function AppSidebar({ page, onPageChange }: { page: string; onPageChange: (page: string) => void }) {
  return <Sidebar><SidebarContent><SidebarGroup><SidebarGroupLabel>ChatGPT MCP</SidebarGroupLabel><SidebarGroupContent><SidebarMenu>{items.map((item) => <SidebarMenuItem key={item.id}><SidebarMenuButton isActive={page === item.id} onClick={() => onPageChange(item.id)}><item.icon />{item.title}</SidebarMenuButton></SidebarMenuItem>)}</SidebarMenu></SidebarGroupContent></SidebarGroup></SidebarContent></Sidebar>
}
