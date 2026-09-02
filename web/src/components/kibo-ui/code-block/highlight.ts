import { transformerNotationDiff, transformerNotationErrorLevel, transformerNotationFocus, transformerNotationHighlight, transformerNotationWordHighlight } from "@shikijs/transformers"
import { createHighlighterCore } from "shiki/core"
import { createJavaScriptRegexEngine } from "shiki/engine/javascript"
import json from "shiki/langs/json.mjs"
import toml from "shiki/langs/toml.mjs"
import yaml from "shiki/langs/yaml.mjs"
import githubDarkDefault from "shiki/themes/github-dark-default.mjs"
import githubLight from "shiki/themes/github-light.mjs"
import type { CodeBlockLanguage } from "./index"

const highlighter = createHighlighterCore({ langs: [...json, ...yaml, ...toml], themes: [githubLight, githubDarkDefault], engine: createJavaScriptRegexEngine() })
const transformers = [transformerNotationDiff({ matchAlgorithm: "v3" }), transformerNotationHighlight({ matchAlgorithm: "v3" }), transformerNotationWordHighlight({ matchAlgorithm: "v3" }), transformerNotationFocus({ matchAlgorithm: "v3" }), transformerNotationErrorLevel({ matchAlgorithm: "v3" })]

export async function highlightCode(code: string, language: CodeBlockLanguage = "json") {
  return (await highlighter).codeToHtml(code, { lang: language, themes: { light: "github-light", dark: "github-dark-default" }, transformers })
}
