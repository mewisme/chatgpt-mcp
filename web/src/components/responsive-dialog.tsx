import type { ReactNode } from "react"
import { useIsMobile } from "@/hooks/use-mobile"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Drawer, DrawerContent, DrawerDescription, DrawerFooter, DrawerHeader, DrawerTitle } from "@/components/ui/drawer"
import { cn } from "@/lib/utils"

type Props = { open: boolean; onOpenChange: (open: boolean) => void; title: string; description?: string; children: ReactNode; footer?: ReactNode; className?: string; wide?: boolean }

export function ResponsiveDialog({ open, onOpenChange, title, description, children, footer, className, wide = false }: Props) {
  const mobile = useIsMobile()
  if (mobile) return <Drawer open={open} onOpenChange={onOpenChange}><DrawerContent className="max-h-[92vh] min-w-0 overflow-hidden"><DrawerHeader><DrawerTitle>{title}</DrawerTitle>{description ? <DrawerDescription>{description}</DrawerDescription> : null}</DrawerHeader><div className="min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto"><div className={cn("min-w-0 max-w-full px-4 pb-4", className)}>{children}</div></div>{footer ? <DrawerFooter className="border-t">{footer}</DrawerFooter> : null}</DrawerContent></Drawer>
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className={cn("max-h-[85vh] min-w-0 overflow-hidden sm:max-w-2xl", wide && "sm:max-w-4xl")}><DialogHeader className="min-w-0"><DialogTitle>{title}</DialogTitle>{description ? <DialogDescription>{description}</DialogDescription> : null}</DialogHeader><div className="min-h-0 min-w-0 max-w-full max-h-[65vh] overflow-x-hidden overflow-y-auto"><div className={cn("min-w-0 max-w-full pr-3", className)}>{children}</div></div>{footer ? <DialogFooter>{footer}</DialogFooter> : null}</DialogContent></Dialog>
}