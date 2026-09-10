# Editor Field Types

## Text input

Single-line value entry. Guidance that explains what to type is placed in the input placeholder rather than a separate sublabel where possible. Validation errors remain attached to the field/editor feedback. Backspace belongs to the active input and must not trigger global Back navigation.

## Password input

Single-line secret/sensitive entry using password echo behavior. Secret-preserving editors explicitly document when blank means “keep existing”; blank is not automatically equivalent to clearing a secret.

## Multiline text

Textarea-style content for lists, rules, JSON, command patterns, environment assignments, and other multiline values. Enter inserts a newline. Multiline Enter never triggers editor completion; mutating editors still use `Ctrl+S` for their explicit save/action.

## Select

Finite-choice field. Options map display labels to persisted values. Use arrow/navigation keys supported by Huh/Bubbles to choose a value.

## Switch

Persistent boolean field with explicit true/false labels. Space/mouse toggles the value. Enter remains field traversal; only an editor explicitly configured for non-mutating completion turns final-field traversal into its primary action.

## Path field

Composite picker/manual-input field. Picker mode is preferred when choosing an existing file/directory; `Ctrl+O` switches to manual input. Both modes bind the same draft value, so switching modes does not discard the path. Field options may constrain file vs directory, root boundaries, relative output, missing-path allowance, and custom validation.

## Validation and save

Mutating editors never rely on “last field completes the form”: validation runs before the page's mutation and `Ctrl+S` is the explicit primary action. Non-mutating editors may opt into final-field Enter completion while reusing the same validation path. Backend/validation failure keeps the draft. Successful mutation must call the editor acceptance/rebase behavior (or rebuild/clear the editor) before navigation so the global dirty guard does not ask to discard already-saved data.
