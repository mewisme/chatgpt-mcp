import {
  transformerNotationDiff,
  transformerNotationErrorLevel,
  transformerNotationFocus,
  transformerNotationHighlight,
  transformerNotationWordHighlight,
} from "@shikijs/transformers"
import type { BundledLanguage, CodeOptionsMultipleThemes } from "shiki"
import { codeToHtml } from "shiki"

export function highlightCode(
  code: string,
  language?: BundledLanguage,
  themes?: CodeOptionsMultipleThemes["themes"]
) {
  return codeToHtml(code, {
    lang: language ?? "typescript",
    themes: themes ?? { light: "github-light", dark: "github-dark-default" },
    transformers: [
      transformerNotationDiff({ matchAlgorithm: "v3" }),
      transformerNotationHighlight({ matchAlgorithm: "v3" }),
      transformerNotationWordHighlight({ matchAlgorithm: "v3" }),
      transformerNotationFocus({ matchAlgorithm: "v3" }),
      transformerNotationErrorLevel({ matchAlgorithm: "v3" }),
    ],
  })
}
