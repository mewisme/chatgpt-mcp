# Global Rule Fields

Global Rules are managed instruction rules displayed in the Instruction → Rules tab. Create/edit uses a routed full-page editor.

## Name

Human-readable rule name. New rules start with `New rule`; existing rules load their persisted name.

## Enabled

Persistent boolean controlling whether the rule is active. Disabling a rule keeps its name/content stored while excluding it from active rule behavior.

## Content

Multiline rule/instruction content. The textarea grows within the editor's available terminal height (with bounded minimum/maximum lines) and follows wrapping-first behavior.

## Rule identity

The rule ID is generated when creating a rule and is not an editable field. Existing rule routes use the stable ID to locate the rule. Saving commits the editor baseline before returning to Rules, preventing the global dirty guard from treating the successful save as an unsaved draft.
