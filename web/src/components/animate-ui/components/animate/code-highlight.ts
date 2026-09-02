import { createHighlighterCore } from "shiki/core"
import { createJavaScriptRegexEngine } from "shiki/engine/javascript"
import json from "shiki/langs/json.mjs"
import toml from "shiki/langs/toml.mjs"
import yaml from "shiki/langs/yaml.mjs"
import githubDark from "shiki/themes/github-dark.mjs"
import githubLight from "shiki/themes/github-light.mjs"

export type CodeLanguage = "json" | "yaml" | "toml"

const highlighter = createHighlighterCore({ langs: [...json, ...yaml, ...toml], themes: [githubLight, githubDark], engine: createJavaScriptRegexEngine() })

export async function highlightCode(code: string, lang: CodeLanguage, dark: boolean) {
  return (await highlighter).codeToHtml(code, { lang, theme: dark ? "github-dark" : "github-light" })
}
