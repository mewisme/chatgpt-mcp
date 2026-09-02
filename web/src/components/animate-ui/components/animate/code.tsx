import * as React from "react"
import { CopyIcon } from "lucide-react"
import { useTheme } from "next-themes"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

type CodeContextType = { code: string }

const CodeContext = React.createContext<CodeContextType | null>(null)

function useCode() {
  const context = React.useContext(CodeContext)
  if (!context) throw new Error("Code components must be used within Code")
  return context
}

type CodeProps = React.ComponentProps<"div"> & { code: string }

function Code({ className, code, ...props }: CodeProps) {
  return <CodeContext.Provider value={{ code }}><div className={cn("relative flex flex-col overflow-hidden rounded-lg border bg-accent/50", className)} {...props} /></CodeContext.Provider>
}

type CodeHeaderProps = React.ComponentProps<"div"> & { icon?: React.ElementType; copyButton?: boolean; copyLabel?: string }

function CodeHeader({ className, children, icon: Icon, copyButton = false, copyLabel = "Copy code", ...props }: CodeHeaderProps) {
  const { code } = useCode()
  const copy = () => void navigator.clipboard.writeText(code)
  return <div className={cn("flex h-10 w-full shrink-0 items-center gap-x-2 border-b border-border/75 bg-accent px-4 text-sm text-muted-foreground dark:border-border/50", className)} {...props}>{Icon && <Icon className="size-4" />}{children}{copyButton && <Button type="button" variant="ghost" size="icon-xs" className="ml-auto -mr-2" aria-label={copyLabel} onClick={copy}><CopyIcon /></Button>}</div>
}

type CodeBlockProps = React.ComponentProps<"div"> & { lang: string }

function CodeBlock({ className, lang, ...props }: CodeBlockProps) {
  const { resolvedTheme } = useTheme()
  const { code } = useCode()
  const [html, setHtml] = React.useState("")

  React.useEffect(() => {
    let active = true
    void import("shiki").then(({ codeToHtml }) => codeToHtml(code, { lang, theme: resolvedTheme === "dark" ? "github-dark" : "github-light" })).then((value) => { if (active) setHtml(value) }).catch(console.error)
    return () => { active = false }
  }, [code, lang, resolvedTheme])

  return <div className={cn("relative overflow-auto p-4 text-sm [&_code]:!text-[13px] [&_code_.line]:!px-0 [&>pre]:!bg-transparent [&>pre]:border-none [&_code]:!bg-transparent", className)} dangerouslySetInnerHTML={html ? { __html: html } : undefined} {...props}>{html ? null : <pre><code>{code}</code></pre>}</div>
}

export { Code, CodeBlock, CodeHeader, type CodeBlockProps, type CodeHeaderProps, type CodeProps }
