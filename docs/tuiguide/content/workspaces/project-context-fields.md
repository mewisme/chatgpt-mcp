# Project Context Builder Fields

The Workspace Project Context builder controls one context-build invocation. It is split into **Scope**, **Budgets**, and **Include** sections. Building validates every numeric budget and keeps the draft if validation/build fails.

## Scope

### Path

Optional path inside the workspace root used to narrow the context build. Empty uses the workspace root/default scope. The value is normalized by Project Context logic and must remain within the workspace access model.

### Memory query

Optional relevance query used when memory is included. It lets the memory selection step bias retrieved memory toward the current task/topic instead of relying only on generic workspace memory.

## Budgets

Every budget is an integer validated against the min/max constants exported by the Project Context package.

### Max memory entries

Maximum number of memory entries included in the build. The editor rejects values outside `projectcontext.MinMemoryEntries` through `projectcontext.MaxMemoryEntries`.

### Max memory bytes

Maximum byte budget for selected memory content. Must stay within the Project Context package's supported memory-byte range.

### Max instruction bytes

Maximum total instruction-context byte budget produced by the build.

### Max section bytes

Per-section byte cap used to prevent one instruction/context section from consuming the whole build budget.

### Max lines per section

Maximum line count retained per context section.

## Include

### Git

Include or exclude Git-derived Project Context information.

### Memory

Include or exclude workspace memory. When excluded, Memory query has no effect on the build.

### Skills

Include or exclude detected project skill metadata/instructions in the Project Context result.

## Build result

`Ctrl+S`/the editor primary action builds Project Context with these options. Success commits the builder draft and opens the preview session. Failure keeps the draft and displays feedback. The preview has Rendered, Sources, and JSON views and follows the same wrapping-first rule as the rest of the TUI.
