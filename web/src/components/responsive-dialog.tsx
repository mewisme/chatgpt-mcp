import type { ReactNode } from "react"
import { useIsMobile } from "@/hooks/use-mobile"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Drawer, DrawerContent, DrawerDescription, DrawerFooter, DrawerHeader, DrawerTitle } from "@/components/ui/drawer"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"

type Props = { open: boolean; onOpenChange: (open: boolean) => void; title: string; description?: string; children: ReactNode; footer?: ReactNode; className?: string; wide?: boolean }

export function ResponsiveDialog({ open, onOpenChange, title, description, children, footer, className, wide = false }: Props) {
  const mobile = useIsMobile()
  if (mobile) return <Drawer open={open} onOpenChange={onOpenChange}><DrawerContent className="max-h-[92vh]"><DrawerHeader><DrawerTitle>{title}</DrawerTitle>{description ? <DrawerDescription>{description}</DrawerDescription> : null}</DrawerHeader><ScrollArea className="min-h-0 flex-1"><div className={cn("px-4 pb-4", className)}>{children}</div></ScrollArea>{footer ? <DrawerFooter className="border-t">{footer}</DrawerFooter> : null}</DrawerContent></Drawer>
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className={cn("max-h-[85vh] sm:max-w-2xl", wide && "sm:max-w-4xl")}><DialogHeader><DialogTitle>{title}</DialogTitle>{description ? <DialogDescription>{description}</DialogDescription> : null}</DialogHeader><ScrollArea className="min-h-0 max-h-[65vh]"><div className={cn("pr-3", className)}>{children}</div></ScrollArea>{footer ? <DialogFooter>{footer}</DialogFooter> : null}</DialogContent></Dialog>
}